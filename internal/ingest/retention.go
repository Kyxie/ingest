package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Release is an explicit assertion by the owner that no share needs this batch anymore.
func (e *Engine) Release(id string) error {
	if !validID(id) {
		return fmt.Errorf("invalid batch ID")
	}
	var b Batch
	if err := readJSON(e.manifest(id), &b); err != nil {
		return err
	}
	return writeJSON(filepath.Join(e.Config.State, "released", id+".json"), time.Now().UTC())
}
func (e *Engine) Prune(ctx context.Context) error {
	unlock, err := e.Lock("scan")
	if err != nil {
		return err
	}
	defer unlock()
	bs, err := e.Batches()
	if err != nil {
		return err
	}
	for _, b := range bs {
		if !b.Complete || b.Pruned || time.Since(b.Created) < time.Duration(e.Config.RetentionDays)*24*time.Hour {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Config.State, "released", b.ID+".json")); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		var r Receipt
		if err := readJSON(e.receipt(b.ID), &r); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		ready := true
		for _, f := range b.Files {
			if f.Archive && r.Verified[f.Path] != f.SHA256 {
				ready = false
			}
		}
		if !ready {
			continue
		}
		// Validate the entire batch before removing anything. Remove only inventoried paths.
		for _, f := range b.Files {
			for _, p := range []string{e.original(b.ID, f.Path), e.shared(b.ID, f.Path)} {
				if _, err := os.Lstat(p); os.IsNotExist(err) {
					continue
				} else if err != nil {
					return err
				}
				sum, err := digest(ctx, p)
				if err != nil {
					return err
				}
				if sum != f.SHA256 {
					return fmt.Errorf("retention refuses modified file: %s", p)
				}
			}
		}
		for _, f := range b.Files {
			for _, p := range []string{e.shared(b.ID, f.Path), e.original(b.ID, f.Path)} {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
		b.Pruned = true
		if err := writeJSON(e.manifest(b.ID), b); err != nil {
			return err
		}
	}
	return nil
}
