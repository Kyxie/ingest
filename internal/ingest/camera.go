package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var nikonZfcFolder = regexp.MustCompile(`(?i)^[0-9]{3}NZ_FC$`)

func isNikonZfc(root string, files []File) bool {
	if root != "" {
		marker := filepath.Join(root, "NIKON", "Z_FC")
		if err := noSymlink(marker); err == nil {
			if info, err := os.Stat(marker); err == nil && info.IsDir() {
				return true
			}
		}
	}
	for _, f := range files {
		parts := strings.Split(f.Path, string(filepath.Separator))
		if len(parts) > 2 && strings.EqualFold(parts[0], "DCIM") && nikonZfcFolder.MatchString(parts[1]) {
			return true
		}
	}
	return false
}

func validViewPath(p string) bool {
	dir, name := filepath.Dir(p), filepath.Base(p)
	return filepath.IsLocal(p) && (dir == "raw" || dir == "jpg") && name != "." && filepath.Join(dir, name) == p
}

func (e *Engine) classifyNikon(b *Batch, batches []Batch, firstSeen time.Time) error {
	used := map[string]bool{}
	for _, candidate := range batches {
		used[candidate.View] = true
	}
	loc, err := time.LoadLocation(e.Config.ImportTimezone)
	if err != nil {
		return err
	}
	base := firstSeen.In(loc).Format("20060102150405")
	for n, name := 1, base; ; n, name = n+1, fmt.Sprintf("%s-%02d", base, n+1) {
		if used[name] {
			continue
		}
		if _, err := os.Lstat(e.shared(name, ".")); os.IsNotExist(err) {
			b.View = name
			break
		} else if err != nil {
			return err
		}
	}
	names := map[string]bool{}
	for i := range b.Files {
		f := &b.Files[i]
		category := ""
		switch strings.ToLower(filepath.Ext(f.Path)) {
		case ".nef":
			category = "raw"
		case ".jpg", ".jpeg":
			category = "jpg"
		default:
			continue
		}
		base := filepath.Base(f.Path)
		ext := filepath.Ext(base)
		for n, name := 1, base; ; n, name = n+1, fmt.Sprintf("%s-%02d%s", strings.TrimSuffix(base, ext), n+1, ext) {
			p := filepath.Join(category, name)
			key := strings.ToLower(p)
			if !names[key] {
				names[key] = true
				f.View = p
				break
			}
		}
	}
	return nil
}

func (e *Engine) publishShares(b Batch) error {
	for _, f := range b.Files {
		for _, p := range e.sharePaths(b, f) {
			if err := shareLink(e.original(b.ID, f.Path), p); err != nil {
				return err
			}
		}
	}
	return nil
}

// PublishNikon adds a classified view to an existing verified Nikon batch.
func (e *Engine) PublishNikon(ctx context.Context, id string) error {
	unlock, err := e.Lock("scan")
	if err != nil {
		return err
	}
	defer unlock()
	batches, err := e.Batches()
	if err != nil {
		return err
	}
	for _, b := range batches {
		if b.ID != id {
			continue
		}
		if !b.Complete || b.Pruned || (b.View == "" && !isNikonZfc("", b.Files)) {
			return fmt.Errorf("batch %q is not a complete Nikon Z fc batch", id)
		}
		for _, f := range b.Files {
			sum, err := digest(ctx, e.original(id, f.Path))
			if err != nil {
				return err
			}
			if sum != f.SHA256 {
				return fmt.Errorf("original changed: %s", f.Path)
			}
		}
		if b.View == "" {
			if err = e.classifyNikon(&b, batches, b.Created); err != nil {
				return err
			}
			if err = writeJSON(e.manifest(id), b); err != nil {
				return err
			}
		}
		return e.publishShares(b)
	}
	return fmt.Errorf("batch %q not found", id)
}
