"""Localens private stdin/stdout worker. No network inference endpoint."""
import json
import sys
import contextlib
import os
from pathlib import Path

os.environ.setdefault("HF_HUB_DISABLE_XET", "1")

# Reuse the project-local cache installed alongside the virtual environment.
if sys.prefix != sys.base_prefix:
    os.environ.setdefault("HF_HOME", str(Path(sys.prefix).parent / ".models"))


def reply(value):
    print(json.dumps(value, ensure_ascii=False, allow_nan=False), flush=True)


def main():
    try:
        # Libraries sometimes print progress to stdout; preserve stdout for JSONL.
        with contextlib.redirect_stdout(sys.stderr):
            import torch
            from sentence_transformers import SentenceTransformer
            from PIL import Image
            # Float16 is explicitly unsafe for this model. CPU float32 is portable.
            model = SentenceTransformer(
                os.environ.get("LOCALENS_MODEL_PATH", "google/embeddinggemma-2"),
                local_files_only=bool(os.environ.get("LOCALENS_MODEL_PATH")),
                device="cpu",
                config_kwargs={"audio_config": None},
                model_kwargs={"torch_dtype": torch.float32},
            )
        reply({"ready": True})
    except Exception as exc:
        reply({"error": f"模型加载失败：{type(exc).__name__}: {exc}"})
        return
    for line in sys.stdin:
        try:
            request = json.loads(line)
            kind = request["kind"]
            options = {"truncate_dim": 256, "normalize_embeddings": True, "show_progress_bar": False}
            with contextlib.redirect_stdout(sys.stderr), torch.inference_mode():
                if kind == "image":
                    with Image.open(request["path"]) as image:
                        image = image.convert("RGB")
                        vector = model.encode({"image": image}, **options)
                elif kind == "query":
                    vector = model.encode(request["text"], prompt_name="SearchQuery", **options)
                else:
                    vector = model.encode(request["text"], **options)
            vector = vector.reshape(-1).tolist()
            if len(vector) != 256:
                raise ValueError(f"Expected 256 dimensions, got {len(vector)}")
            reply({"vector": vector})
        except Exception as exc:
            reply({"error": f"{type(exc).__name__}: {exc}"})


if __name__ == "__main__":
    main()
