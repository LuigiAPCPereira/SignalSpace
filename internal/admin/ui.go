package admin

import (
	"embed"
	"net/http"
)

//go:embed ui/index.html ui/admin.js
var adminUI embed.FS

func serveAdminUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	name := "ui/index.html"
	if r.URL.Path == "/admin.js" {
		name = "ui/admin.js"
	}
	data, err := adminUI.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if name == "ui/admin.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = w.Write(data)
}
