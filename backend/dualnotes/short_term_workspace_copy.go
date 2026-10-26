package dualnotes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"panel/store"
)

func (s *Server) handleCopyWorkspace(w http.ResponseWriter, r *http.Request) {
	if _, err := store.ParseWorkspaceMonth(r.PathValue("sourceMonth")); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	var input struct {
		OperationID    *string  `json:"operationId"`
		TargetMonth    *string  `json:"targetMonth"`
		SourceRevision *int64   `json:"sourceRevision"`
		TargetRevision *int64   `json:"targetRevision"`
		TableIDs       []string `json:"tableIds"`
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
	if input.OperationID == nil || input.TargetMonth == nil || input.SourceRevision == nil || input.TargetRevision == nil || input.TableIDs == nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("operationId、targetMonth、sourceRevision、targetRevision 和 tableIds 为必填项"))
		return
	}
	result, err := s.store.CopyWorkspaceTables(r.Context(), r.PathValue("sourceMonth"), store.WorkspaceCopyRequest{
		OperationID: *input.OperationID, TargetMonth: *input.TargetMonth,
		SourceRevision: *input.SourceRevision, TargetRevision: *input.TargetRevision, TableIDs: input.TableIDs,
	})
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
