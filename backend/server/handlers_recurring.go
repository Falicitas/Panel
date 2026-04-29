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
	Content string `json:"content"`
	Kind    string `json:"kind"`
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
	if p.Kind == "" {
		p.Kind = store.RecurringKindFree
	}
	if !store.ValidRecurringKind(p.Kind) {
		writeError(w, http.StatusBadRequest, errors.New("invalid kind"))
		return
	}
	item, err := s.store.CreateRecurring(p.Content, p.Kind)
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
	if p.Kind == "" {
		p.Kind = store.RecurringKindFree
	}
	if !store.ValidRecurringKind(p.Kind) {
		writeError(w, http.StatusBadRequest, errors.New("invalid kind"))
		return
	}
	if err := s.store.UpdateRecurring(id, p.Content, p.Kind); err != nil {
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

// handleReorderRecurring body: {"kind": "morning", "ids": [3, 1, 2]}
func (s *Server) handleReorderRecurring(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var p struct {
		Kind string  `json:"kind"`
		IDs  []int64 `json:"ids"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !store.ValidRecurringKind(p.Kind) {
		writeError(w, http.StatusBadRequest, errors.New("invalid kind"))
		return
	}
	if err := s.store.ReorderRecurring(p.Kind, p.IDs); err != nil {
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
