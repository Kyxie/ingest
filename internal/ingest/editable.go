package ingest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func (e *Engine) editableImportsMarker() string {
	return filepath.Join(e.Config.Target, "shared", ".imports-editable-v1")
}

func (e *Engine) editableImportsReady() (bool, error) {
	info, err := os.Lstat(e.editableImportsMarker())
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("editable imports marker is not a regular file")
	}
	return true, nil
}

// prepareEditableImports runs under the scan lock. File Browser waits for the marker
// before serving the writable import tree.
func (e *Engine) prepareEditableImports(ctx context.Context) error {
	if !e.Config.EditableImports {
		return nil
	}
	if ready, err := e.editableImportsReady(); ready || err != nil {
		return err
	}
	root := e.shared(".", ".")
	if err := noSymlink(root); err != nil {
		return err
	}
	err := filepath.WalkDir(root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".ingest-detach-") {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Nlink < 2 {
			return nil
		}
		return detachImport(ctx, p, info)
	})
	if err != nil {
		return err
	}
	return writeJSON(e.editableImportsMarker(), true)
}

func detachImport(ctx context.Context, p string, info os.FileInfo) error {
	sum, err := digest(ctx, p)
	if err != nil {
		return err
	}
	parent := filepath.Dir(p)
	tmp, err := os.MkdirTemp(parent, ".ingest-detach-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	copyPath := filepath.Join(tmp, "copy")
	if err = verifiedCopy(ctx, p, copyPath, File{Size: info.Size(), SHA256: sum, Modified: info.ModTime()}); err != nil {
		return err
	}
	current, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if !os.SameFile(info, current) || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
		return fmt.Errorf("import changed while detaching hardlink: %s", p)
	}
	if err = os.Rename(copyPath, p); err != nil {
		return err
	}
	return syncDir(parent)
}
