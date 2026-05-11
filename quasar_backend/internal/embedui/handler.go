package embedui

import (
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

// Handler monta servidor de ficheiros estáticos com fallback SPA (React Router).
func Handler(log zerolog.Logger) http.Handler {
	spaFS, err := SPAFS()
	if err != nil {
		log.Error().Err(err).Msg("embedui: SPAFS")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "UI não disponível", http.StatusInternalServerError)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		default:
			http.NotFound(w, r)
			return
		}

		p := strings.Trim(strings.TrimPrefix(r.URL.Path, "/"), "/")
		upath := filepath.ToSlash(strings.TrimSuffix(p, "/"))
		if strings.Contains(upath, "..") {
			http.NotFound(w, r)
			return
		}

		serveIdx := func() {
			// Evita que o browser fique com index.html antigo após novo build Docker (ficheiros em /assets/ têm hash novo).
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			http.ServeFileFS(w, r, spaFS, "index.html")
		}

		if upath == "" {
			serveIdx()
			return
		}

		_, statErr := fs.Stat(spaFS, upath)
		if errors.Is(statErr, fs.ErrNotExist) {
			serveIdx()
			return
		}
		if statErr != nil {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(upath, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		}
		http.ServeFileFS(w, r, spaFS, upath)
	})
}
