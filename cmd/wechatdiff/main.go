package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcxiaochenn/WeChatDiff/internal/diff"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "错误：", e)
		os.Exit(1)
	}
}
func run() error {
	oldPath := flag.String("old", "", "旧 APK 路径")
	newPath := flag.String("new", "", "新 APK 路径；为空则从官网下载")
	oldURL := flag.String("old-url", "", "旧 APK 官方地址，仅用于首次历史对比")
	newURL := flag.String("new-url", "", "新 APK 官方地址；为空则解析官网")
	baseline := flag.String("baseline", "", "已发布 snapshot.json.gz")
	out := flag.String("out", "out", "报告目录")
	work := flag.String("work", ".work/run", "临时 APK 和反编译目录")
	jadx := flag.String("jadx", "", "JADX 可执行文件路径")
	apksigner := flag.String("apksigner", "", "apksigner 可执行文件路径；Actions 强制验证")
	known := flag.String("known-tags", "", "已经发布的 SHA-256 tag 列表")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	if e := os.MkdirAll(*work, 0755); e != nil {
		return e
	}
	unique, e := os.MkdirTemp(*work, "analysis-")
	if e != nil {
		return e
	}
	*work = unique
	var old *diff.Snapshot
	if *baseline != "" {
		old, e = diff.Load(*baseline)
		if e != nil {
			return e
		}
	}
	if *newPath == "" {
		if *newURL == "" {
			*newURL, e = diff.Discover(ctx)
			if e != nil {
				return e
			}
		}
		*newPath = filepath.Join(*work, "new.apk")
		fmt.Println("下载新 APK：", *newURL)
		if e = diff.Download(ctx, *newURL, *newPath); e != nil {
			return e
		}
	}
	fmt.Println("读取新 APK 身份与结构：", *newPath)
	next, e := diff.Inspect(*newPath, *newURL)
	if e != nil {
		return e
	}
	if e = diff.VerifySignature(ctx, *apksigner, *newPath, next); e != nil {
		return e
	}
	if *known != "" {
		b, e := os.ReadFile(*known)
		if e != nil {
			return e
		}
		for _, tag := range strings.Fields(string(b)) {
			if tag == next.Identity.Tag() {
				fmt.Println("此 SHA-256 已发布，跳过分析")
				return output("changed", "false")
			}
		}
	}
	if old != nil && old.Identity.SHA256 == next.Identity.SHA256 {
		fmt.Println("SHA-256 未变化，跳过分析")
		return output("changed", "false")
	}
	if *oldPath == "" && old != nil {
		*oldURL = old.Identity.Source
	}
	if *oldPath == "" && *oldURL != "" {
		*oldPath = filepath.Join(*work, "old.apk")
		fmt.Println("恢复旧 APK：", *oldURL)
		if e = diff.Download(ctx, *oldURL, *oldPath); e != nil {
			if old == nil {
				return e
			}
			next.Warnings = append(next.Warnings, "旧 APK 下载失败；采用已发布结构快照，无法比较新旧反编译片段")
			*oldPath = ""
		}
	}
	if *oldPath != "" {
		fmt.Println("读取旧 APK：", *oldPath)
		actual, e := diff.Inspect(*oldPath, *oldURL)
		if e != nil {
			return e
		}
		if e = diff.VerifySignature(ctx, *apksigner, *oldPath, actual); e != nil {
			return e
		}
		if old != nil && actual.Identity.SHA256 != old.Identity.SHA256 {
			next.Warnings = append(next.Warnings, "旧地址内容已变化；采用已发布快照，不将替换包当成旧 APK")
			*oldPath = ""
		} else {
			old = actual
		}
	}
	oldDir, newDir := filepath.Join(*work, "old-code"), filepath.Join(*work, "new-code")
	if old != nil && *oldPath != "" && *jadx != "" {
		fmt.Println("JADX 反编译旧 APK")
		if e = diff.Decompile(ctx, *oldPath, oldDir, *jadx, old); e != nil {
			return e
		}
	}
	if old != nil {
		fmt.Println("JADX / 结构差分分析新 APK")
		if e = diff.Decompile(ctx, *newPath, newDir, *jadx, next); e != nil {
			return e
		}
	} else {
		next.Warnings = append(next.Warnings, "首次基线跳过 JADX，后续对比时恢复旧 APK")
	}
	if *oldPath == "" {
		oldDir = ""
	}
	report, e := diff.Compare(old, next, oldDir, newDir)
	if e != nil {
		return e
	}
	model := os.Getenv("DEEPSEEK_MODEL")
	if model == "" {
		model = "deepseek-flash"
	}
	diff.AnalyzeAI(ctx, report, os.Getenv("DEEPSEEK_API_KEY"), model, "https://api.deepseek.com/chat/completions")
	if e = diff.WriteReport(*out, report, next); e != nil {
		return e
	}
	b, _ := json.Marshal(next.Identity)
	fmt.Println(string(b))
	fmt.Printf("报告完成：%s；证据 %d 条；%s\n", *out, len(report.Evidence), report.AIStatus)
	for _, kv := range [][2]string{{"changed", "true"}, {"tag", next.Identity.Tag()}, {"version", next.Identity.Version}, {"build", next.Identity.Build}} {
		if e = output(kv[0], kv[1]); e != nil {
			return e
		}
	}
	return nil
}
func output(k, v string) error {
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = fmt.Fprintf(f, "%s=%s\n", k, v)
		return e
	}
	return nil
}
