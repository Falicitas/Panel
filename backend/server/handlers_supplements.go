package server

import (
	"errors"
	"net/http"
	"strings"

	"panel/store"
)

func (s *Server) handleListSupplements(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListSupplements()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleCreateSupplement(w http.ResponseWriter, r *http.Request) {
	var sp store.Supplement
	if err := decodeJSON(r, &sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateSupplement(&sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := s.store.CreateSupplement(sp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var sp store.Supplement
	if err := decodeJSON(r, &sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sp.ID = id
	if err := validateSupplement(&sp); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.UpdateSupplement(sp); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, _ := s.store.ListSupplements()
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleDeleteSupplement(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteSupplement(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateSupplement(sp *store.Supplement) error {
	if strings.TrimSpace(sp.Name) == "" {
		return errors.New("name empty")
	}
	if sp.Type != "food" && sp.Type != "supplement" {
		return errors.New("type must be food | supplement")
	}
	for i, w := range sp.Windows {
		if strings.TrimSpace(w.Start) == "" || strings.TrimSpace(w.End) == "" {
			return errors.New("window has empty start/end at idx " + intToStr(i))
		}
	}
	return nil
}

func intToStr(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
