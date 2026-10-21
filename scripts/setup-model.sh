#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if command -v uv >/dev/null 2>&1; then
  if [ ! -x .venv/bin/python ]; then uv venv --python 3.12 .venv; fi
  uv pip install --python .venv/bin/python -r engine/requirements.txt
else
  if [ ! -x .venv/bin/python ]; then python3 -m venv .venv; fi
  .venv/bin/python -m pip install -r engine/requirements.txt
fi
printf '\n开发推理环境已准备：%s/.venv/bin/python\n源码应用会自动发现此环境；完整发行包无需此步骤。\n' "$PWD"
