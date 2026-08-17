package hostsblock

import (
	"slices"
	"strings"
)

// 标记块定界行, 必须与 scripts/panel-hostsblock 中的 BEGIN/END 严格一致。
const (
	beginMark = "# >>> panel-hostsblock >>>"
	endMark   = "# <<< panel-hostsblock <<<"
)

// BlockLines 生成 sites 的期望标记块内容 (不含定界行),
// 与脚本 block 子命令的输出格式严格一致: 每域名 0.0.0.0 + :: 两行。
func BlockLines(sites []string) []string {
	lines := make([]string, 0, len(sites)*2)
	for _, s := range sites {
		lines = append(lines, "0.0.0.0 "+s, ":: "+s)
	}
	return lines
}

// CurrentBlock 提取 hosts 内容里标记块内的行; 无标记块返回 nil,
// 空标记块返回非 nil 空 slice (用于区分「没拦」和「拦了空名单」)。
func CurrentBlock(content string) []string {
	var lines []string
	inBlock, found := false, false
	for _, line := range strings.Split(content, "\n") {
		switch line {
		case beginMark:
			inBlock, found = true, true
			lines = []string{}
		case endMark:
			inBlock = false
		default:
			if inBlock {
				lines = append(lines, line)
			}
		}
	}
	if !found {
		return nil
	}
	return lines
}

// UpToDate 报告 hosts 内容中的标记块是否恰好等于 sites 的期望块。
func UpToDate(content string, sites []string) bool {
	cur := CurrentBlock(content)
	return cur != nil && slices.Equal(cur, BlockLines(sites))
}
