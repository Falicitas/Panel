package server

import (
	"net/http"

	"panel/todolist"
)

const breadcrumbMaxRunes = 60

// focusItem group 数组里单条 focus bullet 的结构 — 跟 focusResp 顶层 Text/LineNo/Breadcrumbs
// 同 schema 但不带 anchored/found, 适合数组场景。mac 端用同一套字段渲染卡片。
type focusItem struct {
	Text        string   `json:"text"`
	LineNo      int      `json:"line_no"`
	Breadcrumbs []string `json:"breadcrumbs"`
}

type focusResp struct {
	Anchored    *string  `json:"anchored"`
	Found       bool     `json:"found"`
	Text        string   `json:"text,omitempty"`
	LineNo      int      `json:"line_no,omitempty"`
	Breadcrumbs []string `json:"breadcrumbs,omitempty"`
	// Group 是所有未完成且带 [[[focus]]] 的项, 按 lineNo asc。仅当 ≥ 2 项时下发 —
	// 单项时 mac UI 不显示切换控件, 跟 fallback 路径行为一致。
	Group []focusItem `json:"group,omitempty"`
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
	//   #1 — 所有未完成且带 [[[focus]]] 的项 (用户显式锁定, 越过 [[[skip]]] 优先级);
	//        picked = group[0], group ≥ 2 时下发让 mac UI 切换
	//   #2 — fallback: 第一个未完成且未 [[[skip]]] 的项 (无 focus 标记时的原行为)
	var focusItems []*todolist.TodoItem
	for i := range tl.Items {
		it := &tl.Items[i]
		if !it.Checked && it.Focus {
			focusItems = append(focusItems, it)
		}
	}

	var picked *todolist.TodoItem
	if len(focusItems) > 0 {
		picked = focusItems[0]
	} else {
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
		resp.Breadcrumbs = makeFocusCrumbs(picked.ParentChain)
		if len(focusItems) >= 2 {
			resp.Group = make([]focusItem, len(focusItems))
			for i, it := range focusItems {
				resp.Group[i] = focusItem{
					Text:        it.Text,
					LineNo:      it.LineNo,
					Breadcrumbs: makeFocusCrumbs(it.ParentChain),
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func makeFocusCrumbs(parentChain []string) []string {
	out := make([]string, len(parentChain))
	for i, c := range parentChain {
		out[i] = todolist.TruncateRunes(c, breadcrumbMaxRunes)
	}
	return out
}
