// Package frontend embeds the desktop UI without exposing developer files.
package frontend

import "embed"

// Assets is the static UI served by Wails.
//
//go:embed index.html *.js *.css
var Assets embed.FS
