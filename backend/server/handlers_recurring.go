package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"panel/store"
)

type recurringPayload struct {
	Content   string `json:"content"`
	SortOrder int    `json:"sort_order"`
}

func (s *Server) handleListRecurring(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListRecurring()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if items == nil {
		items = []store.RecurringTodo{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateRecurring(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p recurringPayload
	if err := json.Unmarshal(body, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p.Content = strings.TrimSpace(p.Content)
	if p.Content == "" {
		writeError(w, http.StatusBadRequest, errors.New("content is empty"))
		return
	}
	item, err := s.store.CreateRecurring(p.Content, p.SortOrder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) handleUpdateRecurring(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p recurringPayload
	if err := json.Unmarshal(body, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p.Content = strings.TrimSpace(p.Content)
	if p.Content == "" {
		writeError(w, http.StatusBadRequest, errors.New("content is empty"))
		return
	}
	if err := s.store.UpdateRecurring(id, p.Content, p.SortOrder); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := s.store.ListRecurring()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeleteRecurring(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteRecurring(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
