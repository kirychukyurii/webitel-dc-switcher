package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var distFS embed.FS

// GetFileSystem returns the embedded UI filesystem
func GetFileSystem() (http.FileSystem, error) {
	// Create a sub-filesystem that starts at the "dist" directory
	// This allows us to serve files relative to dist/ without including "dist" in URLs
	fsys, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	return http.FS(fsys), nil
}

// IsEmbedded returns true if UI files are embedded
func IsEmbedded() bool {
	// Check if dist directory exists in embedded FS
	_, err := distFS.ReadDir("dist")
	return err == nil
}
