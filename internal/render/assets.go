package render

import (
	"embed"
	"io/fs"
)

//go:embed assets
var assetsFS embed.FS

// Assets returns the embedded static files (base.css, mermaid.min.js)
// rooted at their directory, for the viewer's /_assets/ route.
func Assets() fs.FS {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err) // embedded dir is always present
	}
	return sub
}
