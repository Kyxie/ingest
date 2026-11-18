package ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"
)

type Config struct {
	Sources              string            `json:"sources"`
	Target               string            `json:"target"`
	State                string            `json:"state"`
	RequireMount         bool              `json:"require_mount"`
	PollSeconds          int               `json:"poll_seconds"`
	StableSeconds        int               `json:"stable_seconds"`
	RetentionDays        int               `json:"retention_days"`
	Remote               string            `json:"remote"`
	PCS                  string            `json:"pcs"`
	VerifyDownload       bool              `json:"verify_download"`
	UploadTimeoutMinutes int               `json:"upload_timeout_minutes"`
	AutoPrune            bool              `json:"auto_prune"`
	RawExtensions        []string          `json:"raw_extensions"`
	ProxyExtensions      []string          `json:"proxy_extensions"`
	ImportTimezone       string            `json:"import_timezone"`
	ProjectRemotes       map[string]string `json:"project_remotes"`
}

func Load(p string) (Config, error) {
	c := Config{RequireMount: true, PollSeconds: 30, StableSeconds: 10, RetentionDays: 90, Remote: "/ingest", PCS: "BaiduPCS-Go", VerifyDownload: true, UploadTimeoutMinutes: 1440}
	f, err := os.Open(p)
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	err = d.Decode(&c)
	return c, err
}
func (c *Config) validate() error {
	paths := []*string{&c.Sources, &c.Target, &c.State}
	for _, p := range paths {
		if !filepath.IsAbs(*p) || filepath.Clean(*p) == "/" {
			return fmt.Errorf("paths must be absolute and not root: %q", *p)
		}
		*p = filepath.Clean(*p)
		if err := noSymlink(*p); err != nil {
			return err
		}
	}
	for i, a := range paths {
		for j, b := range paths {
			if i != j && within(*a, *b) {
				return fmt.Errorf("source, target and state must not overlap")
			}
		}
	}
	if c.PollSeconds < 1 || c.StableSeconds < 0 || c.RetentionDays < 1 {
		return fmt.Errorf("invalid poll, stability or retention interval")
	}
	if !strings.HasPrefix(c.Remote, "/") || strings.Contains(c.Remote, "..") || strings.ContainsAny(c.Remote, "\r\n") {
		return fmt.Errorf("remote must be an absolute netdisk path without '..'")
	}
	if c.UploadTimeoutMinutes < 1 {
		c.UploadTimeoutMinutes = 1440
	}
	if _, err := time.LoadLocation(c.ImportTimezone); err != nil {
		return fmt.Errorf("invalid import timezone: %w", err)
	}
	if c.PCS == "" {
		c.PCS = "BaiduPCS-Go"
	}
	if len(c.RawExtensions) == 0 {
		c.RawExtensions = []string{".nef", ".dng", ".arw", ".cr2", ".cr3", ".raf", ".orf", ".rw2", ".nrw"}
	}
	if len(c.ProxyExtensions) == 0 {
		c.ProxyExtensions = []string{".lrf", ".thm"}
	}
	return nil
}
func within(root, p string) bool {
	return p == root || strings.HasPrefix(p, root+string(os.PathSeparator))
}
func noSymlink(p string) error {
	for p != "/" && p != "." {
		s, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && s.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path rejected: %s", p)
		}
		p = filepath.Dir(p)
	}
	return nil
}
func extensionIn(p string, exts []string) bool {
	ext := filepath.Ext(p)
	for _, v := range exts {
		if strings.EqualFold(ext, v) {
			return true
		}
	}
	return false
}
