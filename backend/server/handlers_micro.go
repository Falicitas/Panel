package server

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Server) handleGetMicroToday(w http.ResponseWriter, r *http.Request) {
	tl, err := s.micro.LoadToday()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tl)
}

func (s *Server) handlePutMicroToday(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var payload struct {
		RawMD string `json:"raw_md"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.micro.WriteToday(payload.RawMD); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tl, err := s.micro.LoadToday()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tl)
}
