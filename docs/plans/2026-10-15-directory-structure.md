# Directory structure refactor

Decision: retain a thin root main.go for Wails' root-package build convention. Move the Wails-bound App and lifecycle orchestration into internal/app, grouping search, files, indexing and model operations into separate files. Move platform-native window, tray, menu, hotkey and volume adapters into internal/desktop. That package receives callbacks/context rather than importing the application, keeping dependencies one-way.

The frontend package owns embedded UI assets; the engine package owns the embedded worker. Update the browser binding namespace from main.App to app.App. Move all build/model setup entry scripts under scripts and validation records under docs. Existing catalog and embedding packages retain ownership of indexing/ranking/evidence and inference protocol; no empty search facade is introduced.

Trade-off: a root main.go remains to keep existing Wails CLI configuration and native builds working. Avoid a duplicate cmd/shiwa entry and avoid changing runtime data paths, model assets or preferences. C/Objective-C bridges remain alongside their owning Go package so CGO compiles their native sources correctly.

Validation: preserve existing Go tests in the owning packages, verify bundled resource paths, native shortcut cancellation/events, frontend bridge/API and embedded resources. Run formatting, Go vet/race/native tests, JS/Python regressions, macOS production build/signing and Windows x64/ARM64 cross-builds. Native Linux/Windows runtime behavior remains dependent on their configured CI runners.
