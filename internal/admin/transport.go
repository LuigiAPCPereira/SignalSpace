package admin

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	PublicAddress = "127.0.0.1:7676"
	AdminAddress  = "127.0.0.1:7677"
	AdminOrigin   = "http://localhost:7677"
)

// Listeners reserva as duas portas antes de qualquer publicação externa.
// A origem pública é sempre a 7676; nenhum listener aceita bind em interfaces externas.
type Listeners struct {
	Public net.Listener
	Admin  net.Listener
}

func ReserveListeners() (*Listeners, error) {
	public, err := net.Listen("tcp4", PublicAddress)
	if err != nil {
		return nil, err
	}
	admin, err := net.Listen("tcp4", AdminAddress)
	if err != nil {
		_ = public.Close()
		return nil, err
	}
	return &Listeners{Public: public, Admin: admin}, nil
}

func (l *Listeners) Close() error {
	if l == nil {
		return nil
	}
	var errs []error
	if l.Admin != nil {
		errs = append(errs, l.Admin.Close())
	}
	if l.Public != nil {
		errs = append(errs, l.Public.Close())
	}
	return errors.Join(errs...)
}

// Handler envolve exclusivamente o roteador administrativo. Nunca há fallback
// para um handler público nem confiança em localhost como autenticação.
func Handler(api http.Handler) http.Handler {
	if api == nil {
		api = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Vary", "Origin")
		if r.Host != "localhost:7677" || r.URL.IsAbs() || r.URL.RawPath != "" || strings.Contains(r.URL.Path, "//") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != AdminOrigin {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method == http.MethodOptions || (r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != AdminOrigin) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != "/api/admin/v1" && !strings.HasPrefix(r.URL.Path, "/api/admin/v1/") {
			http.NotFound(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}

func NewServer(api http.Handler) *http.Server {
	return &http.Server{
		Addr:              AdminAddress,
		Handler:           Handler(api),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}
