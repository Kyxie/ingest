package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ClassifyOriginals reorganizes a verified Nikon archive without needing the card.
func (e *Engine) ClassifyOriginals(ctx context.Context, id string) error {
	if !validID(id) {
		return fmt.Errorf("invalid batch ID")
	}
	unlockScan, err := e.Lock("scan")
	if err != nil {
		return err
	}
	defer unlockScan()
	unlockUpload, err := e.Lock("upload")
	if err != nil {
		return err
	}
	defer unlockUpload()
	if err = e.prepareEditableImports(ctx); err != nil {
		return err
	}
	batches, err := e.Batches()
	if err != nil {
		return err
	}
	for _, b := range batches {
		if b.ID != id {
			continue
		}
		if !b.Complete || b.Pruned || !isNikonZfc("", b.Files) {
			return fmt.Errorf("batch %q is not a complete Nikon Z fc archive", id)
		}
		return e.classifyStoredOriginals(ctx, b)
	}
	return fmt.Errorf("batch %q not found", id)
}

func (e *Engine) classifyStoredOriginals(ctx context.Context, b Batch) error {
	for _, f := range b.Files {
		sum, err := digest(ctx, e.storedOriginal(b.ID, f))
		if err != nil {
			return err
		}
		if sum != f.SHA256 {
			return fmt.Errorf("original changed: %s", f.Path)
		}
	}

	changed := false
	// Keep the old paths intact until the new paths all exist and the manifest is durable.
	for i, name := range nikonPaths(b.Files, true) {
		f := &b.Files[i]
		if f.Stored != "" {
			continue
		}
		if err := shareLink(e.original(b.ID, f.Path), e.original(b.ID, name)); err != nil {
			return err
		}
		f.Stored = name
		changed = true
	}
	if changed {
		if err := writeJSON(e.manifest(b.ID), b); err != nil {
			return err
		}
	}

	root := e.original(b.ID, ".")
	for _, f := range b.Files {
		old := e.original(b.ID, f.Path)
		current := e.storedOriginal(b.ID, f)
		if old == current {
			continue
		}
		if err := removeLegacyArchivePath(root, old, current); err != nil {
			return err
		}
	}
	return nil
}

func removeLegacyArchivePath(root, old, current string) error {
	if err := noSymlink(old); err != nil {
		return err
	}
	prior, err := os.Stat(old)
	if err == nil {
		now, err := os.Stat(current)
		if err != nil {
			return err
		}
		if !os.SameFile(prior, now) {
			return fmt.Errorf("refusing to remove a different legacy file: %s", old)
		}
		if err = os.Remove(old); err != nil {
			return err
		}
		if err = syncDir(filepath.Dir(old)); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for dir := filepath.Dir(old); dir != root; dir = filepath.Dir(dir) {
		err := os.Remove(dir)
		if errors.Is(err, syscall.ENOTEMPTY) {
			break
		}
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err = syncDir(filepath.Dir(dir)); err != nil {
			return err
		}
	}
	return nil
}
