// Package web serves the short links created by the shortener: GET /s/{code}
// answers with a redirect to the original job URL.
package web

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
)

// LinkResolver turns a short code into the original URL. *shortener.Shortener
// implements it; tests use a fake.
type LinkResolver interface {
	Resolve(ctx context.Context, code string) (string, error)
}

// NewHandler returns the router for the short link service.
func NewHandler(resolver LinkResolver) http.Handler {
	h := &handler{resolver: resolver}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /s/{code}", h.redirect)
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /{$}", h.index)
	return mux
}

type handler struct {
	resolver LinkResolver
}

func (h *handler) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	longURL, err := h.resolver.Resolve(r.Context(), code)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.NotFound(w, r)
	case err != nil:
		log.Printf("resolve short code %q: %v", code, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	default:
		http.Redirect(w, r, longURL, http.StatusFound)
	}
}

func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (h *handler) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("jobtracker short links\nGET /s/{code} redirects to the job posting\n"))
}
