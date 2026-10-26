package dualnotes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"panel/store"
)

func (s *Server) handleGetWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := s.store.GetMonthWorkspace(r.Context(), r.PathValue("month"))
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func (s *Server) handlePutWorkspace(w http.ResponseWriter, r *http.Request) {
	if _, err := store.ParseWorkspaceMonth(r.PathValue("month")); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	// Pointer fields preserve the distinction between an empty string and
	// omitted/null fields, including nested tables and rows.
	var input struct {
		Revision *int64 `json:"revision"`
		Tables   []struct {
			ID     *string `json:"id"`
			Header *string `json:"header"`
			Rows   []struct {
				ID      *string `json:"id"`
				Content *string `json:"content"`
			} `json:"rows"`
		} `json:"tables"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxWorkspaceBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeWorkspaceDecodeError(w, err)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("请求只能包含一个 JSON 对象")
		}
		writeWorkspaceDecodeError(w, err)
		return
	}
	if input.Revision == nil || input.Tables == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("revision 和 tables 为必填项，tables 必须为数组"))
		return
	}
	tables := make([]store.WorkspaceTable, len(input.Tables))
	for i, table := range input.Tables {
		if table.ID == nil || table.Header == nil || table.Rows == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("每张表必须包含字符串 id、header 和数组 rows"))
			return
		}
		rows := make([]store.WorkspaceRow, len(table.Rows))
		for j, row := range table.Rows {
			if row.ID == nil || row.Content == nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("每行必须包含字符串 id 和 content"))
				return
			}
			rows[j] = store.WorkspaceRow{ID: *row.ID, Content: *row.Content}
		}
		tables[i] = store.WorkspaceTable{ID: *table.ID, Header: *table.Header, Rows: rows}
	}
	workspace, err := s.store.SaveMonthWorkspace(r.Context(), r.PathValue("month"), *input.Revision, tables)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

func writeWorkspaceDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("每月内容超过 4 MiB"))
		return
	}
	writeError(w, http.StatusBadRequest, fmt.Errorf("请求 JSON 无效：%w", err))
}

func writeWorkspaceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, store.ErrWorkspaceInvalid) {
		status = http.StatusBadRequest
	} else if errors.Is(err, store.ErrWorkspaceConflict) {
		status = http.StatusConflict
	}
	writeError(w, status, err)
}
