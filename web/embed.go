// Package web embeds the built SPA (web/dist). The placeholder
// index.html ships until the web build (Task 11) fills dist/.
package web

import "embed"

// Dist holds the embedded SPA build output.
//
//go:embed all:dist
var Dist embed.FS
