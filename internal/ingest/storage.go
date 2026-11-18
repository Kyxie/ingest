package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type File struct {
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	SHA256   string    `json:"sha256"`
	Modified time.Time `json:"modified"`
	Archive  bool      `json:"archive"`
	Share    bool      `json:"share"`
	View     string    `json:"view,omitempty"`
	Stored   string    `json:"stored,omitempty"`
}
type Batch struct {
	ID       string    `json:"id"`
	Key      string    `json:"key"`
	Source   string    `json:"source"`
	Created  time.Time `json:"created"`
	Complete bool      `json:"complete"`
	Pruned   bool      `json:"pruned"`
	Files    []File    `json:"files"`
	View     string    `json:"view,omitempty"`
}
type Receipt struct {
	Verified  map[string]string `json:"verified"`
	Uploaded  map[string]string `json:"uploaded"`
	LastError string            `json:"last_error,omitempty"`
	Updated   time.Time         `json:"updated"`
}
type observation struct {
	Stamp     string
	Since     time.Time
	FirstSeen time.Time
}
type Engine struct {
	observed map[string]observation
	Config   Config
	seen     map[string]string
}

func New(c Config) (*Engine, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	for _, p := range []string{c.State, filepath.Join(c.State, "batches"), filepath.Join(c.State, "receipts"), filepath.Join(c.State, "released"), filepath.Join(c.Target, "originals"), filepath.Join(c.Target, "shared", "imports"), filepath.Join(c.Target, "shared", "manual")} {
		if err := os.MkdirAll(p, 0750); err != nil {
			return nil, err
		}
	}
	e := &Engine{Config: c}
	// Once a library has editable imports, preserve that mode for all commands.
	if ready, err := e.editableImportsReady(); err != nil {
		return nil, err
	} else if ready {
		e.Config.EditableImports = true
	}
	return e, nil
}
func (e *Engine) original(id, p string) string {
	return filepath.Join(e.Config.Target, "originals", id, p)
}
func (e *Engine) storedOriginal(id string, f File) string {
	if f.Stored != "" {
		return e.original(id, f.Stored)
	}
	return e.original(id, f.Path)
}
func (e *Engine) shared(id, p string) string {
	return filepath.Join(e.Config.Target, "shared", "imports", id, p)
}
func (e *Engine) sharePaths(b Batch, f File) []string {
	var paths []string
	if f.Share {
		paths = append(paths, e.shared(b.ID, f.Path))
	}
	if f.View != "" {
		paths = append(paths, e.shared(b.View, f.View))
	}
	return paths
}
func (e *Engine) manifest(id string) string {
	return filepath.Join(e.Config.State, "batches", id+".json")
}
func (e *Engine) receipt(id string) string {
	return filepath.Join(e.Config.State, "receipts", id+".json")
}
func validID(s string) bool {
	if len(s) < 1 || len(s) > 120 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func (e *Engine) Batches() ([]Batch, error) {
	entries, err := os.ReadDir(filepath.Join(e.Config.State, "batches"))
	if err != nil {
		return nil, err
	}
	var out []Batch
	for _, v := range entries {
		if filepath.Ext(v.Name()) != ".json" {
			continue
		}
		var b Batch
		if err = readJSON(filepath.Join(e.Config.State, "batches", v.Name()), &b); err != nil {
			return nil, err
		}
		if !validID(b.ID) || v.Name() != b.ID+".json" || (b.View != "" && !validID(b.View)) {
			return nil, fmt.Errorf("invalid batch ID")
		}
		stored := make(map[string]bool)
		for _, f := range b.Files {
			if !filepath.IsLocal(f.Path) || f.Path == "." {
				return nil, fmt.Errorf("invalid batch file path")
			}
			if f.View != "" && (b.View == "" || !validViewPath(f.View)) {
				return nil, fmt.Errorf("invalid batch view path")
			}
			if f.Stored != "" {
				key := strings.ToLower(f.Stored)
				if !validStoredPath(f.Stored) || stored[key] {
					return nil, fmt.Errorf("invalid or duplicate batch archive path")
				}
				stored[key] = true
			}
		}
		out = append(out, b)
	}
	return out, nil
}
func readJSON(p string, v any) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}
func syncDir(p string) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func writeJSON(p string, v any) error {
	if err := noSymlink(p); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		err = json.NewEncoder(f).Encode(v)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), p); err != nil {
		return err
	}
	return syncDir(filepath.Dir(p))
}
func (e *Engine) Lock(name string) (func(), error) {
	p := filepath.Join(e.Config.State, name+".lock")
	if err := noSymlink(p); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another %s worker is running: %w", name, err)
	}
	return func() { f.Close() }, nil
}

type cancellableReader struct {
	ctx context.Context
	r   io.Reader
}

func (r cancellableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func digest(ctx context.Context, p string) (string, error) {
	if err := noSymlink(p); err != nil {
		return "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, cancellableReader{ctx, f})
	return hex.EncodeToString(h.Sum(nil)), err
}
func verifiedCopy(ctx context.Context, src, dst string, want File) error {
	if err := noSymlink(dst); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		sum, err := digest(ctx, dst)
		if err != nil {
			return err
		}
		if sum != want.SHA256 {
			return fmt.Errorf("destination mismatch, refusing overwrite: %s", dst)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dst), ".ingest-*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), cancellableReader{ctx, in})
	if err != nil {
		return err
	}
	if n != want.Size || hex.EncodeToString(h.Sum(nil)) != want.SHA256 {
		return fmt.Errorf("source changed during copy: %s", src)
	}
	if err = out.Chmod(0640); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = os.Chtimes(out.Name(), want.Modified, want.Modified); err != nil {
		return err
	}
	sum, err := digest(ctx, out.Name())
	if err != nil {
		return err
	}
	if sum != want.SHA256 {
		return fmt.Errorf("destination readback mismatch")
	}
	// Linking publishes without overwriting an existing name; temporary file is removed afterwards.
	if err = os.Link(out.Name(), dst); err != nil {
		return err
	}
	return syncDir(filepath.Dir(dst))
}
func shareLink(src, dst string) error {
	if err := noSymlink(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}
	if a, err := os.Stat(dst); err == nil {
		b, err := os.Stat(src)
		if err != nil {
			return err
		}
		if !os.SameFile(a, b) {
			return fmt.Errorf("share destination exists: %s", dst)
		}
		return nil
	}
	if err := os.Link(src, dst); err != nil {
		return fmt.Errorf("share and originals must be on same filesystem: %w", err)
	}
	return syncDir(filepath.Dir(dst))
}
