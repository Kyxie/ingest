package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"
)

// Upload runs only in the uploader container. Each file has an independent durable receipt.
// Downloading the remote file is deliberate: Baidu's multipart MD5 is not always a full-file hash.
func (e *Engine) Upload(ctx context.Context) error {
	unlock, err := e.Lock("upload")
	if err != nil {
		return err
	}
	defer unlock()
	bs, err := e.Batches()
	if err != nil {
		return err
	}
	var failures []error
	for _, b := range bs {
		if !b.Complete || b.Pruned {
			continue
		}
		r := Receipt{}
		if err := readJSON(e.receipt(b.ID), &r); err != nil && !os.IsNotExist(err) {
			return err
		}
		if r.Verified == nil {
			r.Verified = map[string]string{}
		}
		if r.Uploaded == nil {
			r.Uploaded = map[string]string{}
		}
		if err := writeJSON(e.receipt(b.ID), r); err != nil {
			return err
		}
		for _, f := range b.Files {
			if !f.Archive || r.Verified[f.Path] == f.SHA256 || (!e.Config.VerifyDownload && r.Uploaded[f.Path] == f.SHA256) {
				continue
			}
			err = e.uploadFile(ctx, b, f)
			r.Updated = time.Now().UTC()
			r.LastError = ""
			if err != nil {
				r.LastError = err.Error()
				failures = append(failures, fmt.Errorf("%s/%s: %w", b.ID, f.Path, err))
			} else {
				r.Uploaded[f.Path] = f.SHA256
				if e.Config.VerifyDownload {
					r.Verified[f.Path] = f.SHA256
				}
				slog.Info("archive upload complete", "batch", b.ID, "file", f.Path, "verified", e.Config.VerifyDownload)
			}
			if saveErr := writeJSON(e.receipt(b.ID), r); saveErr != nil {
				return saveErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("upload failures: %v", failures)
	}
	return nil
}
func (e *Engine) uploadFile(parent context.Context, b Batch, f File) error {
	ctx, cancel := context.WithTimeout(parent, time.Duration(e.Config.UploadTimeoutMinutes)*time.Minute)
	defer cancel()
	local := e.storedOriginal(b.ID, f)
	sum, err := digest(ctx, local)
	if err != nil {
		return err
	}
	if sum != f.SHA256 {
		return fmt.Errorf("local archive changed")
	}
	remote := path.Join(e.Config.Remote, b.ID, filepath.ToSlash(f.Path))
	// Argument arrays avoid interpreting file names as shell commands.
	if err = e.command(ctx, "upload", "--policy", "overwrite", "--norapid", local, path.Dir(remote)); err != nil {
		return err
	}
	if !e.Config.VerifyDownload {
		return nil
	}
	return e.verifyRemote(ctx, remote, f)
}

func (e *Engine) verifyRemote(ctx context.Context, remote string, f File) error {
	// Keep large readback files on the HDD, never on the container writable layer or SSD.
	verifyDir := filepath.Join(e.Config.Target, ".verify")
	if err := os.MkdirAll(verifyDir, 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(verifyDir, "readback-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := e.command(ctx, "download", "--saveto", dir, "--nocheck", remote); err != nil {
		return err
	}
	p := filepath.Join(dir, path.Base(remote))
	info, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("remote readback missing: %w", err)
	}
	if info.Size() != f.Size {
		return fmt.Errorf("remote readback size mismatch")
	}
	sum, err := digest(ctx, p)
	if err != nil {
		return err
	}
	if sum != f.SHA256 {
		return fmt.Errorf("remote readback SHA-256 mismatch")
	}
	return nil
}
func (e *Engine) command(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, e.Config.PCS, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("BaiduPCS-Go %s failed: %w", args[0], err)
	}
	return nil
}
