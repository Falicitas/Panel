package server

import (
	"encoding/json"
	"io"
	"net/http"
)

type anchorReq struct {
	Name *string `json:"name"`
}

type anchorResp struct {
	Name *string `json:"name"`
}

func (s *Server) handleGetAnchor(w http.ResponseWriter, r *http.Request) {
	name, err := s.store.GetAnchor()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, anchorResp{Name: name})
}

func (s *Server) handlePostAnchor(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req anchorReq
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name != nil {
		if _, err := s.repo.Get(*req.Name); err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
	}
	if err := s.store.SetAnchor(req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, anchorResp{Name: req.Name})
}
