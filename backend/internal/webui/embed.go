package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static
var embeddedDist embed.FS

func Handler(prefix string) http.Handler {
	dist, err := fs.Sub(embeddedDist, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(dist))
	index := serveIndex(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trimmedPrefix := "/" + strings.Trim(strings.TrimSpace(prefix), "/")
		if trimmedPrefix == "/" {
			trimmedPrefix = ""
		}
		webPrefix := trimmedPrefix + "/webui/"
		if r.URL.Path == trimmedPrefix+"/webui" {
			http.Redirect(w, r, webPrefix, http.StatusTemporaryRedirect)
			return
		}
		relative := strings.TrimPrefix(r.URL.Path, webPrefix)
		if relative == "" || strings.HasSuffix(r.URL.Path, "/") {
			index.ServeHTTP(w, r)
			return
		}
		clean := path.Clean(relative)
		if clean == "." || strings.HasPrefix(clean, "../") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(dist, clean); err != nil {
			index.ServeHTTP(w, r)
			return
		}
		fileReq := r.Clone(r.Context())
		fileReq.URL.Path = "/" + clean
		fileServer.ServeHTTP(w, fileReq)
	})
}

func serveIndex(dist fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	})
}
