package server

import (
	"net/http"

	"panel/todolist"
)

const breadcrumbMaxRunes = 60

type focusResp struct {
	Anchored    *string  `json:"anchored"`
	Found       bool     `json:"found"`
	Text        string   `json:"text,omitempty"`
	LineNo      int      `json:"line_no,omitempty"`
	Breadcrumbs []string `json:"breadcrumbs,omitempty"`
}

func (s *Server) handleGetFocus(w http.ResponseWriter, r *http.Request) {
	anchor, err := s.store.GetAnchor()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := focusResp{Anchored: anchor}
	if anchor == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	tl, err := s.repo.Get(*anchor)
	if err != nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	for _, it := range tl.Items {
		if it.Checked || it.Skip {
			continue
		}
		resp.Found = true
		resp.Text = it.Text
		resp.LineNo = it.LineNo
		crumbs := make([]string, len(it.ParentChain))
		for i, c := range it.ParentChain {
			crumbs[i] = todolist.TruncateRunes(c, breadcrumbMaxRunes)
		}
		resp.Breadcrumbs = crumbs
		break
	}
	writeJSON(w, http.StatusOK, resp)
}
