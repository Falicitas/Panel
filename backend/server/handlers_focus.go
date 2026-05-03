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
	// 选择策略:
	//   #1 — 第一个未完成且带 [[[focus]]] 的项 (用户显式锁定, 越过 [[[skip]]] 优先级)
	//   #2 — 第一个未完成且未 [[[skip]]] 的项 (原行为)
	var picked *todolist.TodoItem
	for i := range tl.Items {
		it := &tl.Items[i]
		if it.Checked {
			continue
		}
		if it.Focus {
			picked = it
			break
		}
	}
	if picked == nil {
		for i := range tl.Items {
			it := &tl.Items[i]
			if it.Checked || it.Skip {
				continue
			}
			picked = it
			break
		}
	}
	if picked != nil {
		resp.Found = true
		resp.Text = picked.Text
		resp.LineNo = picked.LineNo
		crumbs := make([]string, len(picked.ParentChain))
		for i, c := range picked.ParentChain {
			crumbs[i] = todolist.TruncateRunes(c, breadcrumbMaxRunes)
		}
		resp.Breadcrumbs = crumbs
	}
	writeJSON(w, http.StatusOK, resp)
}
