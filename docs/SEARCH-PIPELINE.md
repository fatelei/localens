# Bounded keyword retrieval

Preserve filename-exact (12), filename-phrase (10), body-phrase (9), and scattered-term (2) priority. Filename-only prefix results retain score 8. Exact filename/phrase leaders are gathered first. Full-query trigram candidates are sorted as metadata, paged in groups of 512, and verified until 80 leaders exist. Probing stops at 8192 candidates and uses the complete fallback when necessary. The cap never truncates the final result domain. Exhausted single-term candidates are complete because there is no scattered-term tier for one term.

Fallback computes Unicode literal eligibility and phrase rank in one deterministic function call per candidate. A MATERIALIZED metadata CTE prevents repeated scoring and avoids transferring whole bodies into the global sort/result set. The SQL and final Go sort share score, modification-time and path ordering.

After the global page cap, at most four workers load evidence, preserving indexed output order. Every metadata cursor is closed before requesting a body connection. Current name, modification time and literal score are rechecked; obsolete evidence is discarded. Semantic passages verify their chunk ID, path and vector. This avoids stale snippets during indexing while keeping code excerpts and absolute line numbers.

Regression coverage includes multiword phrase priority beyond previous caps, 8194 false-positive candidates beyond the probe budget, Unicode/punctuation, mixed filename/body words, parallel queries, bounded workers, cancellation, deletion and changed evidence. No content schema or index format changes are needed.
