# Optional image OCR

Image OCR is an independent persisted preference (`ocrEnabled`), disabled when absent, including existing installations. Enabling it resumes the existing image queue; disabling it cancels the current result and schedules no further recognition. Apple Vision itself may finish an in-flight native request before the cancellation is observed. Existing recognized text and image vectors are retained. Full-text indexing and semantic search continue independently.

The settings toggle is unavailable on platforms without an OCR implementation. Manual recognition retries require OCR to be enabled. Recognition progress is shown only when enabled; paused state is explicit. Preference changes save atomically, and failed saves retain the previous state.

Validation covers opt-in gating, cancellation without committing text, resuming pending images, preserving searchable OCR text when paused, persistence and platform availability, and the independent settings toggle.
