package httpapi

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// contentSecurityPolicy for the single-page app (ADR 0008): only same-origin
// scripts, styles, fonts and API calls; no inline scripts; no framing.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: blob:; " +
	"font-src 'self'; connect-src 'self'; manifest-src 'self'; worker-src 'self'; object-src 'none'; " +
	"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// spaHandler serves the built web app. Real files are served as they are;
// any other path returns index.html so client-side routes work on reload.
// os.DirFS refuses paths that escape the directory.
func spaHandler(dir string) http.Handler {
	root := os.DirFS(dir)
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=()")
		clean := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if clean != "" {
			if info, err := fs.Stat(root, clean); err == nil && !info.IsDir() {
				// Vite fingerprints asset names, so they may be cached for long.
				if strings.HasPrefix(clean, "assets/") || strings.HasPrefix(clean, "fonts/") {
					h.Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// The service worker and index must always be revalidated.
		h.Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		files.ServeHTTP(w, r2)
	})
}
