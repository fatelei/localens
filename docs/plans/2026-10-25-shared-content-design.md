# Shared content storage

Files retain their identity, metadata and semantic chunks. Extracted text moves to content-addressed Unicode fragments referenced by each file in sequence. Identical stored fragments share text and lexical postings. A file-facing SQL view reconstructs exactly the original body, preserving snippets, code line numbers and existing offset-based semantic chunks.

Content-defined boundaries target 8K Unicode characters (minimum 4K, maximum 16K). Each later fragment includes up to 499 preceding characters so every supported literal query (maximum 500 characters) is contained in an indexed fragment. References skip the overlap during reconstruction. This trades bounded overlap for exact cross-boundary phrase search without falling back to a full catalog scan. Small files naturally share whole bodies. A SHA-256 digest identifies a fragment; matching bytes are checked before references are created.

Reference deletion collects only fragments with no remaining references, including their lexical postings. Vectors remain per file because the embedding input includes the filename. Empty bodies, OCR text, Unicode, duplicate fragments within one file, root deletion and cancelled migrations are covered by tests.

Migration runs offline in a transaction, preserves chunk IDs/vectors and metadata, and retains the original database as a rollback backup before the installed application is switched. An analysis command measures the existing text, unique fragment text, reference count and overlap overhead without writing the source index. Compaction is a separate offline operation after successful migration and semantic/lexical/body verification. No original user files are modified or uploaded.
