package hostsblock

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// domainRe 域名白名单字符集。同时是 shell 注入防线: 名单内容会拼进
// sudo 命令行 (经 zsh -c), 任何元字符都必须拒绝。脚本侧有同样校验, 双保险。
var domainRe = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// defaultSitesTemplate 名单文件不存在时写入的初始模板, 用户直接编辑。
const defaultSitesTemplate = `# panel 早间拦截名单 — 后端启动后的拦截窗口内屏蔽这些网站
# 每行一个域名 (自动一并拦截 www. 变体), # 开头为注释
# 修改后下次后端启动生效
bilibili.com
youtube.com
`

// LoadSites 读拦截名单; 文件不存在则先写入默认模板再解析。
// 返回: 归一化 (小写/去重/补 www. 变体, 保序) 的域名列表 + 被跳过的非法行。
func LoadSites(path string) (sites []string, skipped []string, err error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if werr := os.WriteFile(path, []byte(defaultSitesTemplate), 0o644); werr != nil {
			return nil, nil, fmt.Errorf("写默认名单 %s: %w", path, werr)
		}
		raw = []byte(defaultSitesTemplate)
	} else if err != nil {
		return nil, nil, fmt.Errorf("读名单 %s: %w", path, err)
	}

	seen := map[string]bool{}
	add := func(d string) {
		if !seen[d] {
			seen[d] = true
			sites = append(sites, d)
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !domainRe.MatchString(line) || !strings.Contains(line, ".") {
			skipped = append(skipped, line)
			continue
		}
		add(line)
		if !strings.HasPrefix(line, "www.") {
			add("www." + line)
		}
	}
	return sites, skipped, nil
}
