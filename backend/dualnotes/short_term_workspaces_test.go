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

func TestWorkspaceHTTPRoundTripAndConflict(t *testing.T) {
	h := dualNotesHandler(t)
	path := "/api/short-term-workspaces/2026-10"
	response := dualNotesRequest(t, h, "GET", path, "", http.StatusOK)
	var workspace store.MonthWorkspace
	if err := json.Unmarshal(response.Body.Bytes(), &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Month != "2026-10" || workspace.Revision != 0 || workspace.Tables == nil || len(workspace.Tables) != 0 {
		t.Fatalf("empty month contract: %+v", workspace)
	}
	tables := []store.WorkspaceTable{
		{ID: "z-first", Header: "列标题", Rows: []store.WorkspaceRow{{ID: "row", Content: "## Markdown\n\n```go\nfmt.Println(\"a | b\")\n```"}, {ID: "blank", Content: ""}}},
		{ID: "a-second", Header: "", Rows: []store.WorkspaceRow{{ID: "row", Content: "允许跨表复用行 ID"}}},
		{ID: "empty", Header: "无行的表", Rows: []store.WorkspaceRow{}},
	}
	body, err := json.Marshal(map[string]any{"revision": 0, "tables": tables})
	if err != nil {
		t.Fatal(err)
	}
	response = dualNotesRequest(t, h, "PUT", path, string(body), http.StatusOK)
	if err := json.Unmarshal(response.Body.Bytes(), &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Month != "2026-10" || workspace.Revision != 1 || !reflect.DeepEqual(workspace.Tables, tables) {
		t.Fatalf("save contract: %+v", workspace)
	}
	saved := dualNotesRequest(t, h, "GET", path, "", http.StatusOK).Body.String()
	dualNotesRequest(t, h, "PUT", path, `{"revision":0,"tables":[]}`, http.StatusConflict)
	if got := dualNotesRequest(t, h, "GET", path, "", http.StatusOK).Body.String(); got != saved {
		t.Fatalf("conflict changed month: %s", got)
	}
	response = dualNotesRequest(t, h, "PUT", path, `{"revision":1,"tables":[]}`, http.StatusOK)
	if err := json.Unmarshal(response.Body.Bytes(), &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Revision != 2 || workspace.Tables == nil || len(workspace.Tables) != 0 {
		t.Fatalf("versioned empty: %+v", workspace)
	}
	dualNotesRequest(t, h, "PUT", path, string(body), http.StatusConflict)
	other := dualNotesRequest(t, h, "GET", "/api/short-term-workspaces/2026-11", "", http.StatusOK)
	if err := json.Unmarshal(other.Body.Bytes(), &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Revision != 0 || len(workspace.Tables) != 0 {
		t.Fatalf("months are coupled: %+v", workspace)
	}
}

func TestWorkspaceHTTPBadInputDoesNotOverwrite(t *testing.T) {
	h := dualNotesHandler(t)
	path := "/api/short-term-workspaces/2026-10"
	dualNotesRequest(t, h, "PUT", path, `{"revision":0,"tables":[{"id":"keep","header":"保留标题","rows":[{"id":"row","content":"保留内容"}]}]}`, http.StatusOK)
	before := dualNotesRequest(t, h, "GET", path, "", http.StatusOK).Body.String()
	bodies := []string{
		``, `null`, `[]`, `{}`, `{"tables":[]}`, `{"revision":null,"tables":[]}`,
		`{"revision":1}`, `{"revision":1,"tables":null}`, `{"revision":-1,"tables":[]}`,
		`{"revision":9223372036854775807,"tables":[]}`, `{"revision":1.5,"tables":[]}`, `{"revision":"1","tables":[]}`,
		`{"revision":1,"tables":[]} {}`, `{"revision":1,"tables":[],"extra":true}`,
		`{"revision":1,"tables":[null]}`, `{"revision":1,"tables":[{}]}`,
		`{"revision":1,"tables":[{"id":"table","rows":[]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":null,"rows":[]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":false,"rows":[]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":""}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":null}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[],"extra":true}]}`,
		`{"revision":1,"tables":[{"id":"","header":"","rows":[]}]}`,
		`{"revision":1,"tables":[{"id":"same","header":"","rows":[]},{"id":"same","header":"","rows":[]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[null]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[{}]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[{"id":"row"}]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[{"id":"row","content":null}]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[{"id":"row","content":7}]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[{"id":"row","content":"","extra":true}]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[{"id":"","content":""}]}]}`,
		`{"revision":1,"tables":[{"id":"table","header":"","rows":[{"id":"same","content":""},{"id":"same","content":""}]}]}`,
	}
	for i, body := range bodies {
		t.Run(fmt.Sprintf("invalid_body_%d", i), func(t *testing.T) { dualNotesRequest(t, h, "PUT", path, body, http.StatusBadRequest) })
	}
	for _, month := range []string{"2026-1", "2026-00", "2026-13", "0000-01", "2026-10-01", "10000-01"} {
		dualNotesRequest(t, h, "GET", "/api/short-term-workspaces/"+month, "", http.StatusBadRequest)
		dualNotesRequest(t, h, "PUT", "/api/short-term-workspaces/"+month, `{"revision":0,"tables":[]}`, http.StatusBadRequest)
	}
	large := `{"revision":1,"tables":[{"id":"table","header":"","rows":[{"id":"row","content":"` + strings.Repeat("x", store.MaxWorkspaceBytes) + `"}]}]}`
	dualNotesRequest(t, h, "PUT", path, large, http.StatusRequestEntityTooLarge)
	for _, count := range []int{store.MaxWorkspaceTables + 1, store.MaxWorkspaceRows + 1} {
		tables := []store.WorkspaceTable{{ID: "table", Rows: []store.WorkspaceRow{}}}
		if count == store.MaxWorkspaceTables+1 {
			tables = make([]store.WorkspaceTable, count)
			for i := range tables {
				tables[i] = store.WorkspaceTable{ID: fmt.Sprintf("t-%d", i), Rows: []store.WorkspaceRow{}}
			}
		} else {
			for i := 0; i < count; i++ {
				tables[0].Rows = append(tables[0].Rows, store.WorkspaceRow{ID: fmt.Sprintf("r-%d", i)})
			}
		}
		body, err := json.Marshal(map[string]any{"revision": 1, "tables": tables})
		if err != nil {
			t.Fatal(err)
		}
		dualNotesRequest(t, h, "PUT", path, string(body), http.StatusBadRequest)
	}
	after := dualNotesRequest(t, h, "GET", path, "", http.StatusOK).Body.String()
	if after != before {
		t.Fatalf("bad request changed month: before=%s after=%s", before, after)
	}
}
