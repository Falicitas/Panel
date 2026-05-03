package todolist

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	headingRe    = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	listRe       = regexp.MustCompile(`^(\s*)([-*+])\s+(.*)$`)
	taskRe       = regexp.MustCompile(`^\[([ xX])\]\s*(.*)$`)
	skipRe       = regexp.MustCompile(`\[\[\[skip\]\]\]`)
	focusRe      = regexp.MustCompile(`\[\[\[focus\]\]\]`)
	inlineCodeRe = regexp.MustCompile("`[^`]*`")
	// GFM 可转义的 ASCII 标点 (CommonMark spec §6.1)
	mdEscapeRe = regexp.MustCompile("\\\\([!\"#$%&'()*+,\\-./:;<=>?@\\[\\\\\\]^_`{|}~])")
)

// unescapeMarkdown 把 \~ \_ \* 等 markdown 转义还原成字面字符,
// 用于把 raw md 文本变成"给人看"的形态 (focus card / breadcrumb 等)。
func unescapeMarkdown(s string) string {
	return mdEscapeRe.ReplaceAllString(s, "$1")
}

// hasSkipMarker 判断 task 文本是否带 [[[skip]]] 标签。
// 反引号内的 [[[skip]]] 视为字面引用 (例如解释性的 markdown 段落),不算真标记。
func hasSkipMarker(taskText string) bool {
	cleaned := inlineCodeRe.ReplaceAllString(taskText, "")
	return skipRe.MatchString(cleaned)
}

// hasFocusMarker 同 hasSkipMarker, 检测 [[[focus]]]。
// 用于聚焦卡显式锁定到某个 todoitem (越过 [[[skip]]] 优先级)。
func hasFocusMarker(taskText string) bool {
	cleaned := inlineCodeRe.ReplaceAllString(taskText, "")
	return focusRe.MatchString(cleaned)
}

// Parse 解析 markdown 文本,提取所有 - [ ] / - [x] 任务项,
// 同时给每个任务计算 parent_chain (顶层 heading + 上层 list item 的 text 链)。
//
// 多行 task: 后续缩进 >= bullet 内容对齐位 + 非 list/heading + 非空 的行视为
// 同一 task 的 lazy continuation。Text 字段只取第一行 (避免 UI 显示一长段),
// 但 [[[skip]]] / [[[focus]]] marker 检测扫整段合并文本 — 否则用户把 marker
// 写在第二/第三行会被漏掉。
func Parse(content string) []TodoItem {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	type frame struct {
		indent int
		text   string
	}
	var headings []string
	var headingLevels []int
	var listStack []frame

	items := []TodoItem{}
	i := 0
	for i < len(lines) {
		line := lines[i]

		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			text := unescapeMarkdown(strings.TrimSpace(m[2]))
			for len(headings) > 0 && headingLevels[len(headingLevels)-1] >= level {
				headings = headings[:len(headings)-1]
				headingLevels = headingLevels[:len(headingLevels)-1]
			}
			headings = append(headings, text)
			headingLevels = append(headingLevels, level)
			listStack = nil
			i++
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
				rawTaskText := strings.TrimSpace(tm[2])

				// 收集延续行: 不是 list / heading / 空行, 且缩进 >= 内容对齐位 (= indent + 2,
				// bullet "* " 占 2 字符) 即视为同一 task 的延续。简化版 (panel 默认 `* ` /
				// `- ` 形态, +2 够用)。空行直接终止 (tight list 行为)。
				contentIndent := indent + 2
				fullParts := []string{rawTaskText}
				j := i + 1
				for j < len(lines) {
					nl := lines[j]
					if strings.TrimSpace(nl) == "" {
						break
					}
					if headingRe.MatchString(nl) {
						break
					}
					if listRe.MatchString(nl) {
						break
					}
					if countLeadingSpaces(nl) < contentIndent {
						break
					}
					fullParts = append(fullParts, strings.TrimSpace(nl))
					j++
				}

				cleanText := unescapeMarkdown(rawTaskText)
				fullText := unescapeMarkdown(strings.Join(fullParts, " "))
				// skip / focus 检测必须在 unescape 之后: Typora 等编辑器会自动给
				// [ 加转义 (\[\[\[skip]]]), 不剥转义则没有连续三个 [, 正则失配。
				skip := hasSkipMarker(fullText)
				focus := hasFocusMarker(fullText)
				items = append(items, TodoItem{
					LineNo:      i + 1,
					Level:       len(listStack),
					Text:        cleanText,
					Checked:     checked,
					Skip:        skip,
					Focus:       focus,
					ParentChain: parent,
				})
				listStack = append(listStack, frame{indent, cleanText})
				i = j // 跳过已并入的延续行
				continue
			}
			listStack = append(listStack, frame{indent, unescapeMarkdown(strings.TrimSpace(rest))})
			i++
			continue
		}
		i++
	}
	return items
}

// countLeadingSpaces 数前导 ASCII 空格数 (tab 暂不参与折算, 跟 listRe 行为一致)。
func countLeadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
		} else {
			break
		}
	}
	return n
}

// TruncateRunes 截断到 maxRunes 个 rune,超出加 "..."。
func TruncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}
