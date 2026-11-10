// Package web embeds the hand-written interface used by the desktop app.
package web

import "embed"

// Files contains only public interface assets; Go source is never served.
//go:embed *.html js/*.js css/*.css images/*
var Files embed.FS
