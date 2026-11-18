package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func mounted(p string) bool {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		v := strings.Fields(line)
		if len(v) > 4 {
			v[4] = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\134`, `\`).Replace(v[4])
			if v[4] == p {
				return true
			}
		}
	}
	return false
}
func (e *Engine) Scan(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := e.Lock("scan")
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.prepareEditableImports(ctx); err != nil {
		return err
	}
	entries, err := os.ReadDir(e.Config.Sources)
	if err != nil {
		return err
	}
	var errs []error
	present := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		root := filepath.Join(e.Config.Sources, entry.Name())
		if e.Config.RequireMount && !mounted(root) {
			continue
		}
		present[root] = true
		if err = e.scanCard(ctx, root); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
		}
	}
	for root := range e.observed {
		if !present[root] {
			delete(e.seen, root)
			delete(e.observed, root)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("import errors: %v", errs)
	}
	return nil
}
func (e *Engine) scanCard(ctx context.Context, root string) error {
	if err := noSymlink(root); err != nil {
		return err
	}
	dcim := ""
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, v := range entries {
		if strings.EqualFold(v.Name(), "DCIM") && v.IsDir() {
			dcim = filepath.Join(root, v.Name())
		}
	}
	if dcim == "" {
		return nil
	}
	var files []File
	err = filepath.WalkDir(dcim, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source symlink rejected: %s", p)
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular source: %s", p)
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, File{Path: rel, Size: info.Size(), Modified: info.ModTime()})
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	// Cheap metadata fingerprint avoids rereading a whole card every polling interval.
	sh := sha256.New()
	for _, f := range files {
		fmt.Fprintf(sh, "%s\x00%d\x00%d\n", f.Path, f.Size, f.Modified.UnixNano())
	}
	stamp := hex.EncodeToString(sh.Sum(nil))
	if e.observed == nil {
		e.observed = map[string]observation{}
	}
	obs := e.observed[root]
	if obs.FirstSeen.IsZero() {
		obs.FirstSeen = time.Now()
	}
	if obs.Stamp != stamp {
		obs.Stamp, obs.Since = stamp, time.Now()
		e.observed[root] = obs
	}
	if time.Since(obs.Since) < time.Duration(e.Config.StableSeconds)*time.Second {
		return nil
	}
	if e.seen == nil {
		e.seen = map[string]string{}
	}
	if e.seen[root] == stamp {
		return nil
	}
	bs, err := e.Batches()
	if err != nil {
		return err
	}
	type previous struct {
		ID     string
		Pruned bool
		File   File
	}
	known := map[string]previous{}
	for _, b := range bs {
		if !b.Complete {
			continue
		}
		for _, f := range b.Files {
			known[f.Path+"\x00"+f.SHA256] = previous{b.ID, b.Pruned, f}
		}
	}
	all := map[string]bool{}
	for i := range files {
		f := &files[i]
		sum, err := digest(ctx, filepath.Join(root, f.Path))
		if err != nil {
			return err
		}
		after, err := os.Stat(filepath.Join(root, f.Path))
		if err != nil {
			return err
		}
		if after.Size() != f.Size || !after.ModTime().Equal(f.Modified) {
			return fmt.Errorf("source changed during scan")
		}
		f.SHA256 = sum
		if _, previouslyImported := known[f.Path+"\x00"+f.SHA256]; !previouslyImported {
			all[strings.ToLower(f.Path)] = true
		}
	}
	var pending []File
	key := sha256.New()
	for _, f := range files {
		if old, ok := known[f.Path+"\x00"+f.SHA256]; ok {
			if !old.Pruned {
				sum, err := digest(ctx, e.storedOriginal(old.ID, old.File))
				if err != nil {
					return err
				}
				if sum != f.SHA256 {
					return fmt.Errorf("previously imported file changed: %s", f.Path)
				}
			}
			continue
		}
		f.Archive = !extensionIn(f.Path, e.Config.ProxyExtensions)
		f.Share = extensionIn(f.Path, []string{".jpg", ".jpeg", ".png", ".mp4", ".mov"})
		if extensionIn(f.Path, []string{".jpg", ".jpeg"}) {
			stem := strings.TrimSuffix(f.Path, filepath.Ext(f.Path))
			for _, raw := range e.Config.RawExtensions {
				if all[strings.ToLower(stem+raw)] {
					f.Archive = false
				}
			}
		}
		pending = append(pending, f)
		fmt.Fprintf(key, "%s\x00%s\n", f.Path, f.SHA256)
	}
	if len(pending) == 0 {
		e.seen[root] = stamp
		return nil
	}
	k := hex.EncodeToString(key.Sum(nil))
	b := Batch{ID: time.Now().UTC().Format("20060102t150405") + "-" + k[:16], Key: k, Source: filepath.Base(root), Created: time.Now().UTC(), Files: pending}
	resuming := false
	for _, old := range bs {
		if !old.Complete && old.Key == k {
			b = old
			resuming = true
			break
		}
	}
	if e.Config.ArchiveOnly {
		// A pending batch must resume at the paths it already recorded.
		if !resuming && isNikonZfc(root, files) {
			for i, name := range nikonPaths(b.Files, true) {
				b.Files[i].Stored = name
			}
		}
		b.View = ""
		for i := range b.Files {
			b.Files[i].Share = false
			b.Files[i].View = ""
		}
	} else if !resuming && isNikonZfc(root, files) {
		if err = e.classifyNikon(&b, bs, obs.FirstSeen); err != nil {
			return err
		}
		for i := range b.Files {
			b.Files[i].Share = false
		}
	}
	if err = writeJSON(e.manifest(b.ID), b); err != nil {
		return err
	}
	for _, f := range b.Files {
		src := filepath.Join(root, f.Path)
		if err = noSymlink(src); err != nil {
			return err
		}
		if err = verifiedCopy(ctx, src, e.storedOriginal(b.ID, f), f); err != nil {
			return err
		}
	}
	// Publish shares only after every original has passed readback verification.
	if err = e.publishShares(ctx, b); err != nil {
		return err
	}
	b.Complete = true
	if err = writeJSON(e.manifest(b.ID), b); err != nil {
		return err
	}
	e.seen[root] = stamp
	slog.Info("import complete", "batch", b.ID, "files", len(b.Files), "source", b.Source)
	return nil
}
