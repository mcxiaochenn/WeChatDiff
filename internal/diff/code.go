package diff

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Decompile(ctx context.Context, apk, dir, tool string, s *Snapshot) error {
	if tool == "" {
		s.Warnings = append(s.Warnings, "未启用 JADX；本次没有反编译代码覆盖")
		return nil
	}
	// 逐 DEX 加载，避免整个微信 APK 的交叉引用索引超出 runner 内存。
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	z, e := zip.OpenReader(apk)
	if e != nil {
		return e
	}
	defer z.Close()
	if e = os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	s.Code = map[string]string{}
	s.codePaths = map[string]string{}
	s.CodeComplete = true
	for index, item := range z.File {
		if !strings.HasSuffix(item.Name, ".dex") {
			continue
		}
		if ctx.Err() != nil {
			s.CodeComplete = false
			s.Warnings = append(s.Warnings, "JADX 总时限已到；仅比较共同成功生成的类")
			break
		}
		dexPath := filepath.Join(dir, fmt.Sprintf("unit-%d.dex", index))
		r, e := item.Open()
		if e != nil {
			return e
		}
		f, e := os.Create(dexPath)
		if e != nil {
			r.Close()
			return e
		}
		_, e = io.Copy(f, r)
		r.Close()
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		unitDir := filepath.Join(dir, fmt.Sprintf("unit-%d", index))
		log, e := os.Create(unitDir + ".log")
		if e != nil {
			return e
		}
		fmt.Println("  反编译 " + item.Name)
		cmd := toolCommand(ctx, tool, "--no-res", "--no-debug-info", "--no-imports", "--comments-level", "none", "--threads-count", "2", "--decompilation-mode", "simple", "-d", unitDir, dexPath)
		cmd.Env = append(os.Environ(), "JAVA_OPTS=-Xmx3g")
		cmd.Stdout = log
		cmd.Stderr = log
		err := cmd.Run()
		log.Close()
		if err != nil {
			s.CodeComplete = false
			s.Warnings = append(s.Warnings, fmt.Sprintf("JADX %s 非零退出，代码覆盖不完整: %v", item.Name, err))
		}
		if ctx.Err() != nil {
			continue
		}
		root := filepath.Join(unitDir, "sources")
		count := 0
		skipped := 0
		e = filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() || !strings.HasSuffix(path, ".java") {
				return nil
			}
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if strings.Contains(string(b), "throw new UnsupportedOperationException(\"Method not decompiled:") {
				s.CodeComplete = false
				skipped++
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			key := filepath.ToSlash(rel)
			if _, exists := s.Code[key]; exists {
				s.CodeComplete = false
				s.Warnings = append(s.Warnings, "跨 DEX 重复类路径，不将其纳入代码比较: "+key)
				s.Code[key] = ""
				delete(s.codePaths, key)
				return nil
			}
			s.Code[key] = hash([]byte(normalizeCode(string(b))))
			s.codePaths[key] = path
			count++
			return nil
		})
		if e != nil || count == 0 {
			s.CodeComplete = false
			s.Warnings = append(s.Warnings, "JADX "+item.Name+" 没有可用代码输出")
		}
		if skipped > 0 {
			s.Warnings = append(s.Warnings, fmt.Sprintf("%s 排除 %d 个包含 JADX 失败占位代码的类", item.Name, skipped))
		}
	}
	for k, v := range s.Code {
		if v == "" {
			delete(s.Code, k)
		}
	}
	s.Warnings = append(s.Warnings, "逐 DEX 反编译降低内存消耗，但跨 DEX 类型解析可能不完整；代码片段需要审阅。")
	return nil
}
func normalizeCode(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/* loaded from:") || strings.HasPrefix(line, "/* renamed from:") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
func codeExcerpt(old, next *Snapshot, path string) string {
	a, _ := os.ReadFile(old.codePaths[path])
	b, _ := os.ReadFile(next.codePaths[path])
	before, after := lineSet(normalizeCode(string(a))), lineSet(normalizeCode(string(b)))
	var out []string
	bytes := 0
	for _, pair := range []struct {
		data       string
		own, other map[string]bool
		prefix     string
	}{{string(a), before, after, "- "}, {string(b), after, before, "+ "}} {
		seen := map[string]bool{}
		for _, line := range strings.Split(normalizeCode(pair.data), "\n") {
			if !pair.other[line] && !seen[line] {
				seen[line] = true
				if len(line) > 300 {
					line = line[:300]
				}
				if bytes+len(line) > 1800 {
					return strings.Join(out, "\n") + "\n[片段截断]"
				}
				out = append(out, pair.prefix+line)
				bytes += len(line)
			}
		}
	}
	return strings.Join(out, "\n")
}
func lineSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, v := range strings.Split(s, "\n") {
		m[v] = true
	}
	return m
}
