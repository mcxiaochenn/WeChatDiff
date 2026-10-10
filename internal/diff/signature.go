package diff

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

func toolCommand(ctx context.Context, tool string, args ...string) *exec.Cmd {
	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(tool), ".bat") {
		return exec.CommandContext(ctx, "cmd.exe", append([]string{"/c", tool}, args...)...)
	}
	return exec.CommandContext(ctx, tool, args...)
}

var signerPattern = regexp.MustCompile(`certificate SHA-256 digest: ([0-9a-fA-F]{64})`)

func VerifySignature(ctx context.Context, tool, path string, s *Snapshot) error {
	if tool == "" {
		s.Warnings = append(s.Warnings, "未运行 apksigner，未验证 APK 签名有效性")
		return nil
	}
	out, e := toolCommand(ctx, tool, "verify", "--print-certs", path).CombinedOutput()
	if e != nil {
		return fmt.Errorf("APK 签名验证失败: %w", e)
	}
	var digests []string
	for _, m := range signerPattern.FindAllStringSubmatch(string(out), -1) {
		digests = append(digests, strings.ToLower(m[1]))
	}
	sort.Strings(digests)
	if len(digests) == 0 {
		return fmt.Errorf("apksigner 没有返回签名证书")
	}
	s.Identity.Signer = strings.Join(digests, ",")
	return nil
}
