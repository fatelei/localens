# Complete offline releases and continuous integration

Official packages include the application, relocatable CPython 3.12.12, CPU-only inference dependencies and the pinned EmbeddingGemma 2 text/vision model. There is no stripped release variant and no first-run model download. Users do not install Python, configure an interpreter or log into Hugging Face. Build runners download dependencies; installed applications perform inference locally, including without internet access in China.

## Supported targets

| System | Architectures | Archive |
| --- | --- | --- |
| Windows | x64 (`amd64`), ARM64 | ZIP |
| Linux | x64 (`amd64`), ARM64 | tar.gz |
| macOS | Apple Silicon (`arm64`) | ZIP containing 拾物.app |

macOS Intel is excluded. All five jobs use native runners. Put the source directory contents at the GitHub repository root, including `.github`, `go.mod` and `scripts`.

Every branch push and pull request runs formatting/syntax checks, actionlint, Go vet/tests, frontend regressions and packaging regressions, then builds the native application. Linux/macOS include race detection. Linux tests use Xvfb because the global shortcut backend uses X11. Routine CI does not upload incomplete application packages.

## Release gates

Push a tag such as `v0.1.0` to run the same checks, prepare the full assets on each native runner and package all five targets. `scripts/runtime_bundle.py` uses a pinned uv version and managed standalone CPython, resolves binary-only CPU dependencies constrained by `engine/requirements-lock.txt`, writes a platform-specific hash lock, installs with required hashes, and downloads model revision `914f7f89142e33e77833254d9c9b90c3cef7303b`. Windows ARM64 lacks a matching upstream torchvision wheel, so that runner builds version 0.29.1 from pinned official commit `1f1a920d685d46dfbe187934b785b7ecb7a182f2` using native MSVC ARM64 tools before resolving/installing the hashed wheel set. This happens on CI, never on the user device. Dependency or model download failures fail the job; there is no incomplete-package fallback. Newly prepared assets retain `requirements-resolved.txt` for auditing platform-specific transitive dependencies. Build infrastructure needs access to the official Python/PyTorch/model hosting services.

`scripts/package_release.py` accepts only complete assets and verifies application/Python architectures. It rebuilds the bundle from known files, includes the model and dependency license notices, and signs embedded macOS binaries before sealing the app. It then exercises the relocated bundled Python through the same worker used by the application. An empty model cache, offline flags and denied socket connections enforce the offline inference test. Query, text and image requests must each return a normalized 256-dimensional vector. Failure prevents archive creation.

Only after every target succeeds does the publication job verify the exact set of five `-full` archives, checksums and inference reports. It publishes archives, `SHA256SUMS` and `.smoke.json` reports. Prerelease tags produce prereleases. Actions are commit-pinned, with weekly Dependabot updates. Only the publication job receives repository write permission.

## Installation and platform requirements

- Windows: extract the ZIP and run `Shiwa.exe`. Keep the adjacent `Resources` directory. The system Microsoft WebView2 Runtime is required for the desktop UI. Packages are unsigned portable archives.
- macOS: extract and move `拾物.app` to Applications. CI builds target macOS 15+ and use an ad-hoc signature; they are not Apple notarized packages.
- Linux: extract the tarball and run `./Shiwa`, keeping `Resources` beside it. Builds target Ubuntu 24.04 with GTK 3, WebKitGTK 4.1 and X11 libraries. Other distributions need compatible libraries. Global shortcuts require X11; native Wayland shortcuts are not implemented.

The model and Python are inside each archive. Base OS desktop libraries remain platform prerequisites. Semantic search automatically loads the bundled model when enabled. Image OCR remains optional and macOS-only; Windows/Linux OCR is not implemented. Audio/video search currently matches filenames.

## Local checks and packaging

Use a current stable Go toolchain, Node.js 24 and Python 3.13 for CI tooling. Linux development dependencies are `build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev xvfb xauth`. Runtime preparation installs its uv tool into a temporary private directory and separately copies pinned CPython 3.12.12 into its private output; it does not use a developer virtual environment.

```sh
python3 scripts/ci.py lint
python3 scripts/ci.py test
python3 scripts/ci.py build
python3 scripts/runtime_bundle.py
python3 scripts/package_release.py --version v0.1.0 --arch arm64
```

Use `python` on Windows and your actual native architecture. Runtime preparation refuses to overwrite existing assets. For an existing prepared bundle, pass `--assets /absolute/path/to/Resources` to the packaging command. Missing assets or an incompatible runtime are errors. Packaging never silently creates a stripped application. Output goes to `dist/`; no personal files or local search databases are included.

The local complete macOS ARM64 package remains usable offline. The GitHub matrix is configured but hosted runs and interactive platform acceptance still require a connected repository. A passing compile alone is not proof of model compatibility.

References: [GitHub-hosted runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners), [Wails prerequisites](https://wails.io/docs/gettingstarted/installation/), [Astral standalone Python](https://docs.astral.sh/python-build-standalone/).
