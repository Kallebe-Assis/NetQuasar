package embedui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedDist embed.FS

// SPAFS retorna o filesystem apontando para os ficheiros em dist (index.html, assets/).
func SPAFS() (fs.FS, error) {
	return fs.Sub(embeddedDist, "dist")
}
