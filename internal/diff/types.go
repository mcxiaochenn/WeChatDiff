package diff

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const Schema = 1

type Identity struct {
	Package string `json:"package"`
	Version string `json:"version"`
	Build   string `json:"build"`
	SHA256  string `json:"sha256"`
	Source  string `json:"source"`
	ABI     string `json:"abi"`
	Signer  string `json:"signer_sha256,omitempty"`
	Size    int64  `json:"size"`
}

func (i Identity) Tag() string { return "apk-" + i.SHA256 }

type Entry struct {
	Hash string `json:"sha256"`
	Size uint64 `json:"size"`
	Kind string `json:"kind"`
}
type Snapshot struct {
	Schema       int               `json:"schema"`
	Identity     Identity          `json:"identity"`
	Files        map[string]Entry  `json:"files"`
	Manifest     map[string]string `json:"manifest"`
	Strings      map[string]string `json:"strings"`
	Native       map[string]string `json:"native"`
	Code         map[string]string `json:"code"`
	CodeComplete bool              `json:"code_complete"`
	codePaths    map[string]string
	Warnings     []string `json:"warnings"`
}
type Evidence struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Change string `json:"change"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Detail string `json:"detail,omitempty"`
}
type Finding struct {
	Category string   `json:"category"`
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence"`
}
type Report struct {
	Schema   int             `json:"schema"`
	Old      *Identity       `json:"old,omitempty"`
	New      Identity        `json:"new"`
	Counts   map[string]int  `json:"counts"`
	Evidence []Evidence      `json:"evidence"`
	Findings []Finding       `json:"findings,omitempty"`
	Warnings []string        `json:"warnings"`
	AIStatus string          `json:"ai_status"`
	Model    string          `json:"model,omitempty"`
	Usage    json.RawMessage `json:"usage,omitempty"`
}

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Save(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path + ".tmp")
	if err != nil {
		return err
	}
	w := gzip.NewWriter(f)
	err = json.NewEncoder(w).Encode(value)
	if e := w.Close(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
func Load(path string) (*Snapshot, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	z, e := gzip.NewReader(f)
	if e != nil {
		return nil, e
	}
	defer z.Close()
	var s Snapshot
	e = json.NewDecoder(z).Decode(&s)
	if e == nil && (s.Schema != Schema || len(s.Identity.SHA256) != 64 || s.Identity.Package != "com.tencent.mm") {
		e = fmt.Errorf("快照格式或包身份不正确")
	}
	return &s, e
}
