package diff

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, path, version, build, message string, compressed bool) {
	t.Helper()
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	files := map[string][]byte{"AndroidManifest.xml": []byte(`<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.tencent.mm" android:versionName="` + version + `" android:versionCode="` + build + `"><uses-permission android:name="android.permission.INTERNET"/><application android:name="com.tencent.mm.App"/></manifest>`), "classes.dex": dexFixture([]string{message}), "assets/data.txt": []byte(message), "META-INF/CERT.SF": []byte(build)}
	for _, name := range []string{"AndroidManifest.xml", "classes.dex", "assets/data.txt", "META-INF/CERT.SF"} {
		h := &zip.FileHeader{Name: name, Method: zip.Store}
		if compressed {
			h.Method = zip.Deflate
		}
		h.SetModTime(time.Unix(1000, 0))
		w, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(files[name]); e != nil {
			t.Fatal(e)
		}
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
func dexFixture(values []string) []byte {
	b := make([]byte, 112+4*len(values))
	copy(b, "dex\n035\x00")
	binary.LittleEndian.PutUint32(b[56:], uint32(len(values)))
	binary.LittleEndian.PutUint32(b[60:], 112)
	for i, v := range values {
		binary.LittleEndian.PutUint32(b[112+i*4:], uint32(len(b)))
		b = append(b, byte(len(v)))
		b = append(b, []byte(v)...)
		b = append(b, 0)
	}
	return b
}
func TestRealPipelineAndNoise(t *testing.T) {
	root := t.TempDir()
	a, b, c := filepath.Join(root, "a.apk"), filepath.Join(root, "b.apk"), filepath.Join(root, "c.apk")
	fixture(t, a, "8.0.78", "3180", "旧版中文提示", false)
	fixture(t, b, "8.0.78", "3180", "旧版中文提示", true)
	fixture(t, c, "8.0.78", "3181", "新版中文提示", true)
	old, e := Inspect(a, "")
	if e != nil {
		t.Fatal(e)
	}
	repacked, e := Inspect(b, "")
	if e != nil {
		t.Fatal(e)
	}
	if old.Identity.SHA256 == repacked.Identity.SHA256 {
		t.Fatal("压缩方式应改变 APK SHA")
	}
	r, e := Compare(old, repacked, "", "")
	if e != nil || len(r.Evidence) != 0 {
		t.Fatalf("ZIP 压缩噪声未过滤: %+v %v", r, e)
	}
	next, e := Inspect(c, "")
	if e != nil {
		t.Fatal(e)
	}
	r, e = Compare(old, next, "", "")
	if e != nil {
		t.Fatal(e)
	}
	if r.New.Build != "3181" || r.New.Version != "8.0.78" || r.Counts["literal:新增"] != 1 || r.Counts["literal:移除"] != 1 || r.Counts["noise:signing"] != 1 {
		t.Fatalf("同版本不同构建或文字变化错误: %+v", r)
	}
	if e = WriteReport(filepath.Join(root, "report"), r, next); e != nil {
		t.Fatal(e)
	}
	saved, e := Load(filepath.Join(root, "report", "snapshot.json.gz"))
	if e != nil || saved.Identity != next.Identity {
		t.Fatalf("快照恢复失败: %v", e)
	}
	for _, ev := range r.Evidence {
		if ev.ID == "" {
			t.Fatal("证据缺少 ID")
		}
	}
}
func TestBaselineAndIdentityGuards(t *testing.T) {
	s := &Snapshot{Identity: Identity{Package: "com.tencent.mm", ABI: "arm64-v8a", Signer: "signer-a"}}
	r, e := Compare(nil, s, "", "")
	if e != nil || len(r.Evidence) > 0 || r.Old != nil {
		t.Fatal("基线不能声称功能变化")
	}
	next := *s
	next.Identity.ABI = "armeabi-v7a"
	if _, e = Compare(s, &next, "", ""); e == nil {
		t.Fatal("未拒绝跨 ABI")
	}
	next.Identity.ABI = s.Identity.ABI
	next.Identity.Signer = "signer-b"
	if _, e = Compare(s, &next, "", ""); e == nil {
		t.Fatal("未拒绝证书变化")
	}
}
func TestDexStringsAndMalformed(t *testing.T) {
	values, e := dexStrings(dexFixture([]string{"聊天变化", "https://example.com"}))
	if e != nil || len(values) != 2 || values[0] != "聊天变化" {
		t.Fatal(values, e)
	}
	b := dexFixture([]string{"good"})
	binary.LittleEndian.PutUint32(b[112:], 0xffffffff)
	if _, e = dexStrings(b); e == nil {
		t.Fatal("未拒绝越界 DEX")
	}
	if decodeMUTF8([]byte{0xed, 0xa0, 0xbd, 0xed, 0xb8, 0x80}) != "😀" {
		t.Fatal("MUTF8 surrogate 解码错误")
	}
}
func TestOfficialSources(t *testing.T) {
	for _, u := range []string{"https://dldir1v6.qq.com/weixin/android/x.apk", "https://weixin.qq.com/"} {
		if !officialURL(u) {
			t.Fatal(u)
		}
	}
	for _, u := range []string{"http://qq.com/x.apk", "https://qq.com.evil.test/x.apk", "https://127.0.0.1/x.apk", "https://user@qq.com/x.apk", "https://qq.com:8443/x.apk"} {
		if officialURL(u) {
			t.Fatal(u)
		}
	}
}
func TestResourceIDNoise(t *testing.T) {
	a := &Snapshot{Identity: Identity{Package: "com.tencent.mm"}, Manifest: map[string]string{"activity/@theme": "@0x7f010001"}}
	b := &Snapshot{Identity: a.Identity, Manifest: map[string]string{"activity/@theme": "@0x7f010002"}}
	r, e := Compare(a, b, "", "")
	if e != nil || r.Counts["manifest-reference:修改"] != 1 || len(AIInput(r)) != 0 {
		t.Fatal("资源引用地址噪声进入 AI", e)
	}
}
func TestAIInputBudgetAndReferences(t *testing.T) {
	r := &Report{}
	for i := 0; i < 100; i++ {
		r.Evidence = append(r.Evidence, Evidence{ID: "E000001", Kind: "code", Path: "com/tencent/mm/Chat.java", Detail: strings.Repeat("中文代码", 300)})
	}
	selected := AIInput(r)
	b, _ := json.Marshal(selected)
	if len(b) > AIInputBudget {
		t.Fatalf("输入预算超限: %d", len(b))
	}
	good := `{"findings":[{"category":"UI变化","summary":"新增字符串可能关联界面文案调整，需要验证","evidence":["E000001"]}]}`
	if _, e := validateFindings(good, selected); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{strings.ReplaceAll(good, "E000001", "E999999"), strings.ReplaceAll(good, "UI变化", "未知类型"), `{"findings":[{"category":"UI变化","summary":"无依据","evidence":[]}]}`} {
		if _, e := validateFindings(s, selected); e == nil {
			t.Fatal("接受无效 AI 证据", s)
		}
	}
	tooMany := strings.Replace(good, `["E000001"]`, `[`+strings.TrimSuffix(strings.Repeat(`"E000001",`, 9), ",")+`]`, 1)
	if _, e := validateFindings(tooMany, selected); e == nil {
		t.Fatal("接受单条超过8个引用的 AI 结论")
	}
}
func TestAIFailureFallback(t *testing.T) {
	for _, status := range []int{429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			r := &Report{Old: &Identity{}, Evidence: []Evidence{{ID: "E000001", Kind: "literal", Path: "resource:聊天功能"}}}
			AnalyzeAI(context.Background(), r, "test-key", "test-model", server.URL)
			if len(r.Findings) != 0 || !strings.Contains(r.AIStatus, "基础报告") {
				t.Fatal(r.AIStatus)
			}
		})
	}
}

func TestAIEndpoint(t *testing.T) {
	for base, want := range map[string]string{
		"":                            "https://api.deepseek.com/chat/completions",
		"https://gateway.example/v1/": "https://gateway.example/v1/chat/completions",
	} {
		got, err := AIEndpoint(base)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	for _, base := range []string{"http://gateway.example", "https://key@gateway.example", "https://gateway.example?key=secret", "https://gateway.example#fragment"} {
		if _, err := AIEndpoint(base); err == nil {
			t.Fatal("接受不安全的 AI 基础地址")
		}
	}
}
func TestAISuccessAndHallucination(t *testing.T) {
	for _, id := range []string{"E000001", "E999999"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Header.Get("Authorization") != "Bearer test-key" {
				t.Error("未发送正确认证")
			}
			var body map[string]any
			if json.NewDecoder(req.Body).Decode(&body) != nil {
				t.Error("JSON 请求错误")
			}
			if body["max_tokens"].(float64) != 1800 {
				t.Error("输出预算错误")
			}
			response := map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"findings":[{"category":"功能调整","summary":"新增字符串可能涉及调整，待验证","evidence":["` + id + `"]}]}`}}}, "usage": map[string]int{"total_tokens": 80}}
			json.NewEncoder(w).Encode(response)
		}))
		r := &Report{Old: &Identity{}, Evidence: []Evidence{{ID: "E000001", Kind: "literal", Path: "resource:聊天功能"}}}
		AnalyzeAI(context.Background(), r, "test-key", "test-model", server.URL)
		server.Close()
		if id == "E000001" && len(r.Findings) != 1 {
			t.Fatal("有效结果未接受", r.AIStatus)
		}
		if id == "E999999" && len(r.Findings) != 0 {
			t.Fatal("幻觉证据未拒绝")
		}
	}
}
func TestCodeAndMarkdownSafety(t *testing.T) {
	if normalizeCode("// noise\n /* renamed from: x */\n  return \"https://qq.com/\";\n") != "return \"https://qq.com/\";" {
		t.Fatal("注释过滤破坏代码")
	}
	r := &Report{New: Identity{Version: "8.0.79"}, Findings: []Finding{{Category: "UI变化", Summary: "[点击](https://evil.test) <script>", Evidence: []string{"E000001"}}}}
	m := Markdown(r)
	if strings.Contains(m, "<script>") || strings.Contains(m, "[点击]") {
		t.Fatal("未转义不可信文本")
	}
}

func TestPartialCodeCoverage(t *testing.T) {
	a := &Snapshot{Identity: Identity{Package: "com.tencent.mm"}, Code: map[string]string{"Shared.java": "old", "OldOnly.java": "a"}, CodeComplete: false}
	b := &Snapshot{Identity: a.Identity, Code: map[string]string{"Shared.java": "new", "NewOnly.java": "b"}, CodeComplete: true}
	r, e := Compare(a, b, "", "")
	if e != nil {
		t.Fatal(e)
	}
	if r.Counts["code:修改"] != 1 || r.Counts["code:覆盖差异"] != 2 || r.Counts["code:新增"] != 0 || r.Counts["code:移除"] != 0 {
		t.Fatal("不完整覆盖被误报为代码新增/移除", r.Counts)
	}
}
