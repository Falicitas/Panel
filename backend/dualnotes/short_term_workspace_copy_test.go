package dualnotes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"panel/store"
)

func TestWorkspaceCopyHTTPAppendAndReplay(t *testing.T) {
	h := dualNotesHandler(t)
	sourcePath, targetPath := "/api/short-term-workspaces/2026-09", "/api/short-term-workspaces/2026-10"
	dualNotesRequest(t, h, "PUT", sourcePath, `{"revision":0,"tables":[{"id":"a","header":"相同标题","rows":[{"id":"row-a","content":"第一张"}]},{"id":"b","header":"相同标题","rows":[]}]}`, http.StatusOK)
	dualNotesRequest(t, h, "PUT", targetPath, `{"revision":0,"tables":[{"id":"keep","header":"当月已有","rows":[{"id":"row","content":"保留"}]}]}`, http.StatusOK)
	sourceBefore := dualNotesRequest(t, h, "GET", sourcePath, "", http.StatusOK).Body.String()
	body := `{"operationId":"http-copy","targetMonth":"2026-10","sourceRevision":1,"targetRevision":1,"tableIds":["b","a"]}`
	response := dualNotesRequest(t, h, "POST", sourcePath+"/copy", body, http.StatusOK)
	var result store.WorkspaceCopyResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OperationID != "http-copy" || result.Target.Month != "2026-10" || result.Target.Revision != 2 || len(result.Target.Tables) != 3 || len(result.CopiedTableIDs) != 2 {
		t.Fatalf("invalid result: %s", response.Body)
	}
	if result.Target.Tables[0].ID != "keep" || result.Target.Tables[0].Rows[0].Content != "保留" || result.Target.Tables[1].Rows[0].Content != "第一张" || len(result.Target.Tables[2].Rows) != 0 || result.Target.Tables[2].Rows == nil {
		t.Fatalf("copy order or empty rows changed: %+v", result.Target)
	}
	if got := dualNotesRequest(t, h, "GET", sourcePath, "", http.StatusOK).Body.String(); got != sourceBefore {
		t.Fatalf("source changed: %s", got)
	}
	// Edit the destination after a successful copy. A retry returns this newer
	// state while retaining the identities of the original copied tables.
	result.Target.Tables[0].Header = "复制后继续编辑"
	update, err := json.Marshal(map[string]any{"revision": 2, "tables": result.Target.Tables})
	if err != nil {
		t.Fatal(err)
	}
	dualNotesRequest(t, h, "PUT", targetPath, string(update), http.StatusOK)
	response = dualNotesRequest(t, h, "POST", sourcePath+"/copy", body, http.StatusOK)
	var replay store.WorkspaceCopyResult
	if err := json.Unmarshal(response.Body.Bytes(), &replay); err != nil {
		t.Fatal(err)
	}
	if replay.Target.Revision != 3 || replay.Target.Tables[0].Header != "复制后继续编辑" || !reflect.DeepEqual(replay.CopiedTableIDs, result.CopiedTableIDs) || len(replay.Target.Tables) != 3 {
		t.Fatalf("retry duplicated content or returned old snapshot: %+v", replay)
	}
	dualNotesRequest(t, h, "POST", sourcePath+"/copy", strings.Replace(body, `"tableIds":["b","a"]`, `"tableIds":["a"]`, 1), http.StatusBadRequest)
}

func TestWorkspaceCopyHTTPBadInputAndConflictDoNotWrite(t *testing.T) {
	h := dualNotesHandler(t)
	sourcePath, targetPath := "/api/short-term-workspaces/2026-09", "/api/short-term-workspaces/2026-10"
	dualNotesRequest(t, h, "PUT", sourcePath, `{"revision":0,"tables":[{"id":"table","header":"源表","rows":[]}]}`, http.StatusOK)
	beforeSource := dualNotesRequest(t, h, "GET", sourcePath, "", http.StatusOK).Body.String()
	beforeTarget := dualNotesRequest(t, h, "GET", targetPath, "", http.StatusOK).Body.String()
	base := map[string]any{"operationId": "test", "targetMonth": "2026-10", "sourceRevision": 1, "targetRevision": 0, "tableIds": []string{"table"}}
	bodies := []string{"", "null", "[]", "{}"}
	for _, key := range []string{"operationId", "targetMonth", "sourceRevision", "targetRevision", "tableIds"} {
		for _, missing := range []bool{true, false} {
			value := make(map[string]any)
			for k, v := range base {
				value[k] = v
			}
			if missing {
				delete(value, key)
			} else {
				value[key] = nil
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			bodies = append(bodies, string(raw))
		}
	}
	for key, values := range map[string][]any{
		"operationId":    {"", " ", strings.Repeat("x", 129), 5},
		"targetMonth":    {"2026-09", "2026-1", "0000-01", "2026-13", 5},
		"sourceRevision": {-1, 1.5, "1"},
		"targetRevision": {-1, 1.5, "0"},
		"tableIds":       {[]string{}, []any{nil}, []int{1}, []string{" "}, []string{strings.Repeat("x", 129)}, []string{"table", "table"}, []string{"table", "missing"}},
	} {
		for _, input := range values {
			value := make(map[string]any)
			for k, v := range base {
				value[k] = v
			}
			value[key] = input
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			bodies = append(bodies, string(raw))
		}
	}
	valid, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	bodies = append(bodies, string(valid)+" {}", strings.TrimSuffix(string(valid), "}")+`,"unknown":true}`)
	for i, body := range bodies {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) { dualNotesRequest(t, h, "POST", sourcePath+"/copy", body, http.StatusBadRequest) })
	}
	dualNotesRequest(t, h, "POST", "/api/short-term-workspaces/2026-9/copy", string(valid), http.StatusBadRequest)
	dualNotesRequest(t, h, "POST", sourcePath+"/copy", strings.Replace(string(valid), `"sourceRevision":1`, `"sourceRevision":0`, 1), http.StatusConflict)
	dualNotesRequest(t, h, "POST", sourcePath+"/copy", strings.Replace(string(valid), `"targetRevision":0`, `"targetRevision":1`, 1), http.StatusConflict)
	large := `{"operationId":"` + strings.Repeat("x", store.MaxWorkspaceBytes) + `"}`
	dualNotesRequest(t, h, "POST", sourcePath+"/copy", large, http.StatusRequestEntityTooLarge)
	if got := dualNotesRequest(t, h, "GET", sourcePath, "", http.StatusOK).Body.String(); got != beforeSource {
		t.Fatalf("bad copy changed source: %s", got)
	}
	if got := dualNotesRequest(t, h, "GET", targetPath, "", http.StatusOK).Body.String(); got != beforeTarget {
		t.Fatalf("bad copy changed target: %s", got)
	}
	// Reusing the operation ID after rejected requests remains possible.
	dualNotesRequest(t, h, "POST", sourcePath+"/copy", string(valid), http.StatusOK)
}
