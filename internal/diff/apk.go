package diff

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/shogo82148/androidbinary"
)

func Inspect(path, source string) (*Snapshot, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return nil, e
	}
	z, e := zip.NewReader(f, st.Size())
	if e != nil {
		return nil, e
	}
	s := &Snapshot{Schema: Schema, Identity: Identity{SHA256: hex.EncodeToString(h.Sum(nil)), Source: source, Size: st.Size()}, Files: map[string]Entry{}, Manifest: map[string]string{}, Strings: map[string]string{}, Native: map[string]string{}, Code: map[string]string{}}
	abis := map[string]bool{}
	for _, item := range z.File {
		if item.FileInfo().IsDir() {
			continue
		}
		if _, ok := s.Files[item.Name]; ok {
			return nil, fmt.Errorf("APK 存在重复 ZIP 路径: %s", item.Name)
		}
		if item.UncompressedSize64 > 512<<20 {
			return nil, fmt.Errorf("ZIP 条目过大: %s", item.Name)
		}
		r, e := item.Open()
		if e != nil {
			return nil, e
		}
		digest := sha256.New()
		_, e = io.Copy(digest, r)
		r.Close()
		if e != nil {
			return nil, e
		}
		kind := fileKind(item.Name)
		s.Files[item.Name] = Entry{hex.EncodeToString(digest.Sum(nil)), item.UncompressedSize64, kind}
		if strings.HasPrefix(item.Name, "lib/") {
			parts := strings.Split(item.Name, "/")
			if len(parts) > 2 {
				abis[parts[1]] = true
			}
		}
		if kind != "manifest" && kind != "dex" && kind != "native" && item.Name != "resources.arsc" {
			continue
		}
		r, e = item.Open()
		if e != nil {
			return nil, e
		}
		b, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			return nil, e
		}
		switch kind {
		case "manifest":
			facts, id, e := manifest(b)
			if e != nil {
				return nil, e
			}
			s.Manifest = facts
			s.Identity.Package = id.Package
			s.Identity.Version = id.Version
			s.Identity.Build = id.Build
		case "dex":
			values, e := dexStrings(b)
			if e != nil {
				return nil, fmt.Errorf("%s: %w", item.Name, e)
			}
			for i, v := range values {
				if usefulString(v) {
					key := "dex:" + v
					if _, ok := s.Strings[key]; !ok {
						s.Strings[key] = fmt.Sprintf("%s:string_id[%d]", item.Name, i)
					}
				}
			}
		case "native":
			if e := nativeFacts(b, item.Name, s.Native); e != nil {
				s.Warnings = append(s.Warnings, item.Name+": "+e.Error())
			}
		default:
			if item.Name == "resources.arsc" {
				values, e := resourceStrings(b)
				if e != nil {
					return nil, e
				}
				for i, v := range values {
					if usefulString(v) {
						s.Strings["resource:"+v] = fmt.Sprintf("resources.arsc:global_string[%d]", i)
					}
				}
			}
		}
	}
	var abi []string
	for a := range abis {
		abi = append(abi, a)
	}
	sort.Strings(abi)
	s.Identity.ABI = strings.Join(abi, ",")
	if s.Identity.Package != "com.tencent.mm" || s.Identity.Version == "" || s.Identity.Build == "" {
		return nil, fmt.Errorf("APK 不是完整微信包或 Manifest 缺少版本")
	}
	if !regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){1,3}$`).MatchString(s.Identity.Version) || !regexp.MustCompile(`^(?:[0-9]+|0x[0-9a-fA-F]+)$`).MatchString(s.Identity.Build) {
		return nil, fmt.Errorf("版本号或构建号格式不合法")
	}
	if _, ok := s.Files["classes.dex"]; !ok {
		return nil, fmt.Errorf("缺少 classes.dex，不是完整 APK")
	}
	return s, nil
}
func fileKind(path string) string {
	switch {
	case path == "AndroidManifest.xml":
		return "manifest"
	case strings.HasSuffix(path, ".dex"):
		return "dex"
	case strings.HasSuffix(path, ".so"):
		return "native"
	case path == "resources.arsc" || strings.HasPrefix(path, "res/") || strings.HasPrefix(path, "r/"):
		return "resource"
	case strings.HasPrefix(path, "META-INF/"):
		return "signing"
	default:
		return "asset"
	}
}
func manifest(b []byte) (map[string]string, Identity, error) {
	if len(b) == 0 {
		return nil, Identity{}, fmt.Errorf("空 Manifest")
	}
	var reader io.Reader = bytes.NewReader(b)
	if !bytes.HasPrefix(bytes.TrimSpace(b), []byte("<")) {
		x, e := androidbinary.NewXMLFile(bytes.NewReader(b))
		if e != nil {
			return nil, Identity{}, e
		}
		reader = x.Reader()
	}
	d := xml.NewDecoder(reader)
	facts := map[string]string{}
	id := Identity{}
	var stack []string
	counts := map[string]int{}
	for {
		t, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, id, e
		}
		switch n := t.(type) {
		case xml.StartElement:
			attrs := map[string]string{}
			for _, a := range n.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				attrs[a.Name.Local] = a.Value
			}
			if n.Name.Local == "manifest" {
				id.Package = attrs["package"]
				id.Version = attrs["versionName"]
				id.Build = attrs["versionCode"]
			}
			segment := n.Name.Local
			if name := attrs["name"]; name != "" {
				segment += "[" + name + "]"
			} else {
				base := strings.Join(stack, "/") + "/" + segment
				counts[base]++
				segment += "[" + strconv.Itoa(counts[base]) + "]"
			}
			stack = append(stack, segment)
			key := strings.Join(stack, "/")
			for k, v := range attrs {
				if k == "versionName" || k == "versionCode" || k == "compileSdkVersion" || k == "compileSdkVersionCodename" || k == "platformBuildVersionCode" || k == "platformBuildVersionName" {
					continue
				}
				facts[key+"/@"+k] = v
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	return facts, id, nil
}
func nativeFacts(b []byte, path string, out map[string]string) error {
	f, e := elf.NewFile(bytes.NewReader(b))
	if e != nil {
		return e
	}
	defer f.Close()
	if text := f.Section(".text"); text != nil {
		v, e := text.Data()
		if e != nil {
			return e
		}
		out[path+":.text"] = hash(v)
	}
	symbols, e := f.DynamicSymbols()
	if e != nil {
		return e
	}
	var names []string
	for _, sym := range symbols {
		if sym.Name != "" && sym.Section != elf.SHN_UNDEF {
			names = append(names, sym.Name)
		}
	}
	sort.Strings(names)
	out[path+":exports"] = strings.Join(names, "\n")
	return nil
}
func usefulString(s string) bool {
	if len(s) < 6 || len(s) > 400 || strings.ContainsRune(s, 0) {
		return false
	}
	if strings.HasPrefix(s, "L") && strings.HasSuffix(s, ";") {
		return strings.Contains(s, "com/tencent/mm/")
	}
	// 短混淆标识、纯数字、资源地址和编译器生成成员不进入语义证据。
	if strings.Contains(s, "$r8$") || strings.Contains(s, "$$ExternalSynthetic") || strings.Contains(s, "SourceFile") {
		return false
	}
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return strings.Contains(s, " ") || strings.Contains(s, "_") || strings.Contains(s, ".") || strings.Contains(s, "/")
}
