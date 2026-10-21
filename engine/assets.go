// Package engine owns the private Python worker embedded in the application.
package engine

import _ "embed"

// WorkerScript is the JSONL inference process installed in the private data directory.
//
//go:embed worker.py
var WorkerScript []byte
