package server

import (
	"log"
	"net/http"

	"panel/micro"
	"panel/store"
	"panel/todolist"
)

type Server struct {
	repo  *todolist.Repo
	store *store.Store
	micro *micro.Repo
}

func New(repo *todolist.Repo, st *store.Store, microRepo *micro.Repo) *Server {
	return &Server{repo: repo, store: st, micro: microRepo}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/todolists", s.handleListTodolists)
	mux.HandleFunc("GET /api/todolists/{name}", s.handleGetTodolist)
	mux.HandleFunc("PUT /api/todolists/{name}", s.handlePutTodolist)
	mux.HandleFunc("GET /api/anchor", s.handleGetAnchor)
	mux.HandleFunc("POST /api/anchor", s.handlePostAnchor)
	mux.HandleFunc("GET /api/focus", s.handleGetFocus)
	mux.HandleFunc("GET /api/micro/today", s.handleGetMicroToday)
	mux.HandleFunc("PUT /api/micro/today", s.handlePutMicroToday)
	mux.HandleFunc("GET /api/recurring", s.handleListRecurring)
	mux.HandleFunc("POST /api/recurring", s.handleCreateRecurring)
	mux.HandleFunc("PUT /api/recurring/{id}", s.handleUpdateRecurring)
	mux.HandleFunc("DELETE /api/recurring/{id}", s.handleDeleteRecurring)
	return logMiddleware(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func logMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}
