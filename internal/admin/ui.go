package admin

import (
	"embed"
	"net/http"
)

//go:embed ui/index.html ui/admin.css ui/admin.js
var adminUI embed.FS

func serveAdminUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := "ui/index.html"
	switch r.URL.Path {
	case "/admin.css":
		name = "ui/admin.css"
	case "/admin.js":
		name = "ui/admin.js"
	}
	data, err := adminUI.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch name {
	case "ui/admin.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case "ui/admin.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = w.Write(data)
}
