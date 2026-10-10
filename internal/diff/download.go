package diff

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func officialURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") && (h == "qq.com" || strings.HasSuffix(h, ".qq.com"))
}
func client() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || !officialURL(r.URL.String()) {
			return fmt.Errorf("拒绝非官方下载重定向")
		}
		return nil
	}}
}

var apkURL = regexp.MustCompile(`https://[A-Za-z0-9._/-]+weixin[0-9]+android[A-Za-z0-9._-]*\.apk`)

func Discover(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://weixin.qq.com/", nil)
	resp, e := client().Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("官网 HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if e != nil {
		return "", e
	}
	body := strings.ReplaceAll(string(b), `\/`, `/`)
	var urls []string
	for _, u := range apkURL.FindAllString(body, -1) {
		if officialURL(u) && strings.Contains(u, "_arm64") {
			urls = append(urls, u)
		}
	}
	// 官网同时列出旧系统专用包；仅选择数值最高的主线 arm64 包。
	sort.Slice(urls, func(i, j int) bool { return versionDigits(urls[i]) > versionDigits(urls[j]) })
	if len(urls) == 0 {
		return "", fmt.Errorf("官网未发现 arm64 APK；停止而不猜测链接")
	}
	return urls[0], nil
}

var versionPattern = regexp.MustCompile(`weixin([0-9]+)android`)

func versionDigits(u string) int {
	var n int
	m := versionPattern.FindStringSubmatch(u)
	if len(m) > 1 {
		fmt.Sscan(m[1], &n)
	}
	return n
}
func Download(ctx context.Context, raw, path string) error {
	if !officialURL(raw) || !strings.HasSuffix(raw, ".apk") {
		return fmt.Errorf("只允许 HTTPS 腾讯 APK 下载地址")
	}
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		last = downloadOnce(ctx, raw, path)
		if last == nil {
			return nil
		}
	}
	return last
}
func downloadOnce(ctx context.Context, raw, path string) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", raw, nil)
	resp, e := client().Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("下载 HTTP %d", resp.StatusCode)
	}
	f, e := os.Create(path + ".part")
	if e != nil {
		return e
	}
	n, e := io.Copy(f, io.LimitReader(resp.Body, (1<<30)+1))
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if n > 1<<30 || n < 1024 {
		return fmt.Errorf("APK 大小不合理: %d", n)
	}
	return os.Rename(path+".part", path)
}
