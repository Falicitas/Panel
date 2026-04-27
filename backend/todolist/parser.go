package todolist

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	headingRe     = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	listRe        = regexp.MustCompile(`^(\s*)([-*+])\s+(.*)$`)
	taskRe        = regexp.MustCompile(`^\[([ xX])\]\s*(.*)$`)
	skipRe        = regexp.MustCompile(`\[\[\[skip\]\]\]`)
	inlineCodeRe  = regexp.MustCompile("`[^`]*`")
)

// hasSkipMarker 判断 task 文本是否带 [[[skip]]] 标签。
// 反引号内的 [[[skip]]] 视为字面引用 (例如解释性的 markdown 段落),不算真标记。
func hasSkipMarker(taskText string) bool {
	cleaned := inlineCodeRe.ReplaceAllString(taskText, "")
	return skipRe.MatchString(cleaned)
}

// Parse 解析 markdown 文本,提取所有 - [ ] / - [x] 任务项,
// 同时给每个任务计算 parent_chain (顶层 heading + 上层 list item 的 text 链)。
func Parse(content string) []TodoItem {
	content = strings.ReplaceAll(content, "\r\n", "\n")

	type frame struct {
		indent int
		text   string
	}
	var headings []string
	var headingLevels []int
	var listStack []frame

	var items []TodoItem
	for i, line := range strings.Split(content, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			text := strings.TrimSpace(m[2])
			for len(headings) > 0 && headingLevels[len(headingLevels)-1] >= level {
				headings = headings[:len(headings)-1]
				headingLevels = headingLevels[:len(headingLevels)-1]
			}
			headings = append(headings, text)
			headingLevels = append(headingLevels, level)
			listStack = nil
			continue
		}

		if m := listRe.FindStringSubmatch(line); m != nil {
			indent := len(m[1])
			rest := m[3]

			for len(listStack) > 0 && listStack[len(listStack)-1].indent >= indent {
				listStack = listStack[:len(listStack)-1]
			}

			parent := make([]string, 0, len(headings)+len(listStack))
			parent = append(parent, headings...)
			for _, f := range listStack {
				parent = append(parent, f.text)
			}

			if tm := taskRe.FindStringSubmatch(rest); tm != nil {
				checked := tm[1] != " "
				taskText := strings.TrimSpace(tm[2])
				skip := hasSkipMarker(taskText)
				items = append(items, TodoItem{
					LineNo:      i + 1,
					Level:       len(listStack),
					Text:        taskText,
					Checked:     checked,
					Skip:        skip,
					ParentChain: parent,
				})
				listStack = append(listStack, frame{indent, taskText})
			} else {
				listStack = append(listStack, frame{indent, strings.TrimSpace(rest)})
			}
			continue
		}
	}
	return items
}

// TruncateRunes 截断到 maxRunes 个 rune,超出加 "..."。
func TruncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}
