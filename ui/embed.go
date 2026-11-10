package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns an fs.FS rooted inside the built UI dist/ directory.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
