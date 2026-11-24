package webassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

func Files() (fs.FS, bool) {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
