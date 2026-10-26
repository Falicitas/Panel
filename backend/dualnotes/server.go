// Package dualnotes serves the dual-column editor and monthly workspaces.
package dualnotes

import (
	"encoding/json"
	"log"
	"net/http"

	"panel/store"
)

type Server struct {
	store *store.Store
}

func New(st *store.Store) *Server { return &Server{store: st} }

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "panel-dual-notes"})
	})
	mux.HandleFunc("GET /api/dual-notes", s.handleGetDualNotes)
	mux.HandleFunc("PUT /api/dual-notes/{date}", s.handlePutDualNotes)
	mux.HandleFunc("GET /api/short-term-workspaces/{month}", s.handleGetWorkspace)
	mux.HandleFunc("PUT /api/short-term-workspaces/{month}", s.handlePutWorkspace)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
