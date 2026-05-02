package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type todoListResp struct {
	Active []listEntry `json:"active"`
	Done   []listEntry `json:"done"`
}

type listEntry struct {
	Name      string `json:"name"`
	TotalTodo int    `json:"total_todo"`
	Done      int    `json:"done"`
	Skipped   int    `json:"skipped"`
}

func (s *Server) handleListTodolists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.repo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp := todoListResp{Active: []listEntry{}, Done: []listEntry{}}
	for _, tl := range lists {
		e := listEntry{Name: tl.Name}
		for _, it := range tl.Items {
			e.TotalTodo++
			if it.Checked {
				e.Done++
			} else if it.Skip {
				e.Skipped++
			}
		}
		if tl.Done {
			resp.Done = append(resp.Done, e)
		} else {
			resp.Active = append(resp.Active, e)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleGetTodolist(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, errBadName)
		return
	}
	tl, err := s.repo.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, tl)
}

// handleCreateTodolist POST /api/todolists body {name}
//   - name 自动补 .md 后缀;不允许 / \ 等路径字符
//   - 已存在 → 409
//   - 创建成功 → 返回新 Todolist
func (s *Server) handleCreateTodolist(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("name empty"))
		return
	}
	if strings.ContainsAny(name, "/\\") {
		writeError(w, http.StatusBadRequest, errors.New("name cannot contain / or \\"))
		return
	}
	if !strings.EqualFold(filepath.Ext(name), ".md") {
		name += ".md"
	}
	path := filepath.Join(s.repo.RootDir(), name)
	if _, err := os.Stat(path); err == nil {
		writeError(w, http.StatusConflict, errors.New("todolist already exists: "+name))
		return
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tl, err := s.repo.Get(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, tl)
}

func (s *Server) handlePutTodolist(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, errBadName)
		return
	}
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
	if err := s.repo.Write(name, payload.RawMD); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	tl, err := s.repo.Get(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, tl)
}
