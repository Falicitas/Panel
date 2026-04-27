package todolist

type Todolist struct {
	Name  string     `json:"name"`
	Path  string     `json:"path"`
	Done  bool       `json:"done"`
	Items []TodoItem `json:"items"`
	RawMD string     `json:"raw_md"`
}

type TodoItem struct {
	LineNo      int      `json:"line_no"`
	Level       int      `json:"level"`
	Text        string   `json:"text"`
	Checked     bool     `json:"checked"`
	Skip        bool     `json:"skip"`
	ParentChain []string `json:"parent_chain"`
}
