// Package explorerassets exposes the committed production Explorer bundle.
// Node is required to rebuild these bytes, never to install or run the Go CLI.
package explorerassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// FS returns a fresh view rooted at explorer/dist.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
