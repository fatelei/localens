# Image result thumbnails

Show a compact 64x48 image in each image result, keeping the filename/path and selection behavior. Preserve existing document/media icons. Load only visible/near-visible image rows, with two concurrent requests. Reuse a bounded in-memory cache keyed by path, size and modification time; do not build a permanent thumbnail database or read every image while searching.

Expose a dedicated thumbnail method through the Wails App. Verify indexed path/scope, reject placeholders and changed files, bound source bytes and decoded dimensions, then return a small JPEG data URL. Use a separate image helper package for bounded decoding/scaling; keep filesystem authorization in app/catalog. A failed/unsupported thumbnail retains the normal image icon. Do not use the full-resolution preview endpoint per row.

Tests cover bounded output, unsupported/corrupt/oversized sources, cancellation, invalidation, indexed-only access and lazy frontend requests/stale results. Keep current full-preview controls. Run native/race/frontend regressions and Windows cross-builds, update the installed full macOS app while preserving its model/runtime, and verify real sunset search rows in the native UI.
