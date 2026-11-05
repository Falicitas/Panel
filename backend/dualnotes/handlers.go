package dualnotes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"panel/store"
)

func (s *Server) handleGetDualNotes(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if len(query["end"]) != 1 || len(query["days"]) != 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("请提供单个 end 和 days 参数"))
		return
	}
	count := 0
	switch query.Get("days") {
	case "1":
		count = 1
	case "10":
		count = 10
	case "11":
		count = 11
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("days 只能是 1、10 或 11"))
		return
	}
	days, err := s.store.ListDualNoteDays(r.Context(), query.Get("end"), count)
	if err != nil {
		writeDualNotesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Days []store.DualNoteDay `json:"days"`
	}{Days: days})
}

func (s *Server) handlePutDualNotes(w http.ResponseWriter, r *http.Request) {
	if _, err := store.ParseDualNoteDate(r.PathValue("date")); err != nil {
		writeDualNotesError(w, err)
		return
	}
	// Pointer fields distinguish missing/null from an intentionally empty cell.
	var input struct {
		Revision *int64 `json:"revision"`
		Rows     []struct {
			ID    *string `json:"id"`
			Left  *string `json:"left"`
			Right *string `json:"right"`
		} `json:"rows"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, store.MaxDualNotesBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeDualNotesDecodeError(w, err)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("请求只能包含一个 JSON 对象")
		}
		writeDualNotesDecodeError(w, err)
		return
	}
	if input.Revision == nil || input.Rows == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("revision 和 rows 为必填项，rows 必须为数组"))
		return
	}
	rows := make([]store.DualNoteRow, len(input.Rows))
	for i, row := range input.Rows {
		if row.ID == nil || row.Left == nil || row.Right == nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("每行必须包含字符串 id、left 和 right"))
			return
		}
		rows[i] = store.DualNoteRow{ID: *row.ID, Left: *row.Left, Right: *row.Right}
	}
	day, err := s.store.SaveDualNoteDay(r.Context(), r.PathValue("date"), *input.Revision, rows)
	if err != nil {
		writeDualNotesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, day)
}

func writeDualNotesDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("每日内容超过 4 MiB"))
		return
	}
	writeError(w, http.StatusBadRequest, fmt.Errorf("请求 JSON 无效：%w", err))
}

func writeDualNotesError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, store.ErrDualNotesInvalid) {
		status = http.StatusBadRequest
	} else if errors.Is(err, store.ErrDualNotesConflict) {
		status = http.StatusConflict
	}
	writeError(w, status, err)
}
