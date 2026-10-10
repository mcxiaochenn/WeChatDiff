package diff

import (
	"fmt"
	"sort"
	"strings"
)

func Compare(old, next *Snapshot, oldDir, newDir string) (*Report, error) {
	r := &Report{Schema: Schema, New: next.Identity, Counts: map[string]int{}, Warnings: append([]string{}, next.Warnings...), AIStatus: "未配置 Key，基础报告"}
	if old == nil {
		r.Warnings = append(r.Warnings, "首次建立基线，没有旧 APK；不推断版本变化")
		return r, nil
	}
	if old.Identity.Package != next.Identity.Package || old.Identity.ABI != next.Identity.ABI {
		return nil, fmt.Errorf("新旧 APK 的包名或 ABI 不一致，拒绝比较")
	}
	r.Old = &old.Identity
	if old.Identity.Signer != "" && next.Identity.Signer != old.Identity.Signer {
		return nil, fmt.Errorf("签名证书发生变化或本次未验证签名，拒绝发布")
	}
	r.Warnings = append(r.Warnings, old.Warnings...)
	add := func(e Evidence) {
		r.Counts[e.Kind+":"+e.Change]++
		e.ID = fmt.Sprintf("E%06d", len(r.Evidence)+1)
		r.Evidence = append(r.Evidence, e)
	}
	for _, path := range unionKeys(old.Files, next.Files) {
		a, ao := old.Files[path]
		b, bo := next.Files[path]
		if ao && bo && a.Hash == b.Hash {
			continue
		}
		kind := b.Kind
		if !bo {
			kind = a.Kind
		}
		if kind == "signing" {
			r.Counts["noise:signing"]++
			continue
		}
		change := "修改"
		if !ao {
			change = "新增"
		}
		if !bo {
			change = "移除"
		}
		add(Evidence{Kind: kind, Path: path, Change: change, Before: a.Hash, After: b.Hash, Detail: fmt.Sprintf("解压字节大小 %d → %d；字节变化不能直接证明功能变化", a.Size, b.Size)})
	}
	for _, pair := range []struct {
		kind string
		a, b map[string]string
	}{{"manifest-fact", old.Manifest, next.Manifest}, {"literal", old.Strings, next.Strings}, {"native-fact", old.Native, next.Native}} {
		for _, key := range unionKeys(pair.a, pair.b) {
			a, ao := pair.a[key]
			b, bo := pair.b[key]
			if pair.kind == "literal" && ao && bo {
				continue
			}
			if a == b {
				continue
			}
			change := "修改"
			if !ao {
				change = "新增"
			}
			if !bo {
				change = "移除"
			}
			e := Evidence{Kind: pair.kind, Path: key, Change: change, Before: a, After: b}
			if pair.kind == "manifest-fact" && strings.HasPrefix(a, "@0x") && strings.HasPrefix(b, "@0x") {
				e.Kind = "manifest-reference"
				e.Detail = "资源 ID 地址变化；可能仅为构建重排，不进入 AI 功能证据"
			}
			if pair.kind == "literal" {
				e.Detail = "字符串存在性变化；可能由混淆、依赖或未启用功能导致，不能证明用户可见功能上线"
			}
			if pair.kind == "native-fact" && strings.HasSuffix(key, ":exports") {
				e.Before = hash([]byte(a))
				e.After = hash([]byte(b))
				e.Detail = limitedSetDiff(a, b, 2000)
			}
			add(e)
		}
	}
	if len(old.Code) > 0 && len(next.Code) > 0 {
		samples := 0
		components := map[string]bool{}
		for _, s := range []*Snapshot{old, next} {
			for key, name := range s.Manifest {
				if strings.HasSuffix(key, "/@name") && strings.HasPrefix(name, "com.tencent.mm.") {
					components[strings.ReplaceAll(name, ".", "/")+".java"] = true
				}
			}
		}
		codeKeys := unionKeys(old.Code, next.Code)
		sort.SliceStable(codeKeys, func(i, j int) bool { return components[codeKeys[i]] && !components[codeKeys[j]] })
		for _, key := range codeKeys {
			a, ao := old.Code[key]
			b, bo := next.Code[key]
			if (!old.CodeComplete || !next.CodeComplete) && (!ao || !bo) {
				r.Counts["code:覆盖差异"]++
				continue
			}
			if a == b {
				continue
			}
			change := "修改"
			if !ao {
				change = "新增"
			}
			if !bo {
				change = "移除"
			}
			e := Evidence{Kind: "code", Path: key, Change: change, Before: a, After: b}
			if samples < 80 && strings.HasPrefix(key, "com/tencent/mm/") && namedClass(key) && oldDir != "" && newDir != "" {
				e.Detail = codeExcerpt(old, next, key)
				samples++
			}
			add(e)
		}
	} else {
		r.Warnings = append(r.Warnings, "新旧代码索引未同时可用；没有进行代码语义比较")
	}
	r.Warnings = append(r.Warnings, "ZIP 时间戳、压缩方式、顺序与签名文件已过滤；DEX 字符串按内容跨分包比较，忽略索引重排；混淆名称与资源 ID 噪声仍可能存在。")
	return r, nil
}
func unionKeys[V any](a, b map[string]V) []string {
	m := map[string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func namedClass(path string) bool {
	parts := strings.Split(path, "/")
	name := strings.TrimSuffix(parts[len(parts)-1], ".java")
	return len(name) > 5 && !strings.Contains(name, "$$")
}
func limitedSetDiff(a, b string, limit int) string {
	aa, bb := lineSet(a), lineSet(b)
	var out []string
	n := 0
	for _, k := range unionKeys(aa, bb) {
		prefix := ""
		if !aa[k] {
			prefix = "+ "
		}
		if !bb[k] {
			prefix = "- "
		}
		if prefix != "" {
			if n+len(k) > limit {
				out = append(out, "[截断]")
				break
			}
			out = append(out, prefix+k)
			n += len(k)
		}
	}
	return strings.Join(out, "\n")
}
