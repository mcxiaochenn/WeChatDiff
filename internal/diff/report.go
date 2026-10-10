package diff

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteReport(dir string, r *Report, s *Snapshot) error {
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	if e := Save(filepath.Join(dir, "snapshot.json.gz"), s); e != nil {
		return e
	}
	if e := Save(filepath.Join(dir, "diff.json.gz"), r); e != nil {
		return e
	}
	metadata := map[string]any{"schema": Schema, "identity": r.New, "previous": r.Old, "tag": r.New.Tag()}
	if e := os.WriteFile(filepath.Join(dir, "metadata.json"), mustJSON(metadata), 0644); e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(Markdown(r)), 0644)
}
func safe(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "`", "&#96;")
	s = strings.ReplaceAll(s, "|", "&#124;")
	s = strings.ReplaceAll(s, "\n", " ⏎ ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "[", "&#91;")
	s = strings.ReplaceAll(s, "]", "&#93;")
	return s
}
func Markdown(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 微信 Android %s（构建 %s）差分报告\n\n", safe(r.New.Version), safe(r.New.Build))
	fmt.Fprintf(&b, "- 包名：%s\n- ABI：%s\n- SHA-256：%s\n- 来源：%s\n- AI 状态：%s\n\n", r.New.Package, r.New.ABI, r.New.SHA256, safe(r.New.Source), safe(r.AIStatus))
	if r.Old != nil {
		fmt.Fprintf(&b, "比较基线：%s / 构建 %s / SHA-256 %s\n\n", safe(r.Old.Version), safe(r.Old.Build), r.Old.SHA256)
	} else {
		b.WriteString("首次建立基线，没有版本差分，不生成新增功能或修复结论。\n\n")
	}
	b.WriteString("## 已确认的静态变化\n\n仅确认安装包中的事实变化，不等同于用户可见功能已上线。\n\n")
	var keys []string
	for k := range r.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "- %s：%d\n", safe(k), r.Counts[k])
	}
	b.WriteString("\n## AI 推测（需要人工或运行时验证）\n\n")
	if len(r.Findings) == 0 {
		b.WriteString("没有经过证据校验的 AI 结论。\n")
	}
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "- **推测 · %s**：%s（证据：%s）\n", safe(f.Category), safe(f.Summary), strings.Join(f.Evidence, ", "))
	}
	b.WriteString("\n## 证据摘录\n\n完整证据及 SHA-256 对应关系见附件 diff.json.gz；本页为有界摘录。\n\n")
	selected := AIInput(r)
	seen := map[string]bool{}
	for _, e := range selected {
		seen[e.ID] = true
	}
	for _, f := range r.Findings {
		for _, id := range f.Evidence {
			seen[id] = true
		}
	}
	if len(selected) == 0 {
		for i, e := range r.Evidence {
			if i >= 20 {
				break
			}
			seen[e.ID] = true
		}
	}
	for _, e := range r.Evidence {
		if !seen[e.ID] {
			continue
		}
		fmt.Fprintf(&b, "- **%s** [%s / %s] %s\n  - 旧：%s\n  - 新：%s\n", e.ID, safe(e.Kind), e.Change, safe(e.Path), safe(e.Before), safe(e.After))
		if e.Detail != "" {
			fmt.Fprintf(&b, "  - 说明：%s\n", safe(e.Detail))
		}
	}
	b.WriteString("\n## 覆盖与限制\n\n")
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "- %s\n", safe(w))
	}
	b.WriteString("- 不分析热补丁、动态配置、服务端灰度；不将 APK 字节变化解释为实际功能修复。\n")
	return b.String()
}
