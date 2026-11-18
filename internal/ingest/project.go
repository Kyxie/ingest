package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

func pcsComponent(s string) bool {
	return s != "" && s != "." && s != ".." && s == strings.TrimSpace(s) &&
		!strings.ContainsAny(s, `/\*?<>|:",`) && strings.IndexFunc(s, unicode.IsControl) < 0
}

func projectDestination(root, name string) (string, error) {
	if !pcsComponent(name) || !strings.HasPrefix(root, "/") || root == "/" || path.Clean(root) != root {
		return "", fmt.Errorf("expected a project name and a configured absolute Baidu directory")
	}
	for _, part := range strings.Split(root[1:], "/") {
		if !pcsComponent(part) {
			return "", fmt.Errorf("invalid Baidu destination component %q", part)
		}
	}
	return path.Join(root, name), nil
}

func projectFiles(ctx context.Context, root string) ([]File, error) {
	if err := noSymlink(root); err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project is not a directory")
	}
	var files []File
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if p == root {
			return nil
		}
		if !pcsComponent(d.Name()) {
			return fmt.Errorf("filename unsupported by BaiduPCS-Go: %q", p)
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("project entry must be a regular file: %s", p)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		sum, err := digest(ctx, p)
		if err != nil {
			return err
		}
		files = append(files, File{Path: rel, Size: info.Size(), Modified: info.ModTime(), SHA256: sum})
		return nil
	})
	return files, err
}

type pcsKind uint8

const (
	pcsMissing pcsKind = iota
	pcsDirectory
	pcsFile
)

// BaiduPCS-Go meta reports errors in stdout even when it exits with status zero.
// Accept only metadata for the exact path or the API's explicit not-found code.
func (e *Engine) pcsStat(ctx context.Context, remote string) (pcsKind, error) {
	cmd := exec.CommandContext(ctx, e.Config.PCS, "meta", remote)
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return pcsMissing, fmt.Errorf("BaiduPCS-Go meta failed for %q: %w", remote, err)
	}
	for _, line := range strings.Split(output.String(), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "目录路径"); ok && strings.TrimSpace(rest) == remote {
			return pcsDirectory, nil
		}
		if rest, ok := strings.CutPrefix(line, "文件路径"); ok && strings.TrimSpace(rest) == remote {
			return pcsFile, nil
		}
		if strings.HasPrefix(line, "获取文件/目录的元信息: 遇到错误, 远端服务器返回错误, 代码: 31066,") {
			return pcsMissing, nil
		}
	}
	return pcsMissing, fmt.Errorf("could not confirm Baidu path %q; check the destination and BaiduPCS-Go login", remote)
}

// UploadProject previews or uploads a curated project independently of archive batches.
func (e *Engine) UploadProject(ctx context.Context, device, project string, apply, merge bool, out io.Writer) error {
	base, ok := e.Config.ProjectRemotes[device]
	if !ok {
		return fmt.Errorf("no Baidu destination configured for device %q", device)
	}
	dest, err := projectDestination(base, project)
	if err != nil {
		return err
	}
	root := e.shared(project, ".")
	files, err := projectFiles(ctx, root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("project %q has no files", project)
	}
	if _, err = fmt.Fprintf(out, "%s -> %s (%d files)\n", root, dest, len(files)); err != nil {
		return err
	}
	for _, f := range files {
		if _, err = fmt.Fprintf(out, "  %s\n", f.Path); err != nil {
			return err
		}
	}
	if !apply {
		return nil
	}

	unlock, err := e.Lock("upload")
	if err != nil {
		return err
	}
	defer unlock()
	kind, err := e.pcsStat(ctx, base)
	if err != nil {
		return err
	}
	if kind != pcsDirectory {
		return fmt.Errorf("configured Baidu directory does not exist: %s", base)
	}
	kind, err = e.pcsStat(ctx, dest)
	if err != nil {
		return err
	}
	if kind == pcsFile {
		return fmt.Errorf("remote project path is a file: %s", dest)
	}
	if kind == pcsDirectory && !merge {
		return fmt.Errorf("remote project already exists: %s; use --merge to add files", dest)
	}
	dirs := map[string]pcsKind{dest: kind}
	already := make(map[string]bool)
	for _, f := range files {
		remote := path.Join(dest, filepath.ToSlash(f.Path))
		parent, err := e.projectRemoteDir(ctx, path.Dir(remote), dirs)
		if err != nil {
			return err
		}
		if parent == pcsMissing {
			continue
		}
		kind, err := e.pcsStat(ctx, remote)
		if err != nil {
			return err
		}
		if kind == pcsDirectory {
			return fmt.Errorf("remote file path is a directory: %s", remote)
		}
		if kind == pcsFile {
			if err = e.verifyProjectRemote(ctx, remote, f); err != nil {
				return fmt.Errorf("remote file conflict at %s: %w", remote, err)
			}
			already[f.Path] = true
		}
	}

	state := filepath.Join(e.Config.State, "project-receipts")
	if err = noSymlink(state); err != nil {
		return err
	}
	if err = os.MkdirAll(state, 0750); err != nil {
		return err
	}
	key := sha256.Sum256([]byte(dest))
	receiptPath := filepath.Join(state, fmt.Sprintf("%x.json", key))
	receipt := projectReceipt{Remote: dest, Verified: make(map[string]string)}
	if err = noSymlink(receiptPath); err != nil {
		return err
	}
	if err = readJSON(receiptPath, &receipt); err != nil && !os.IsNotExist(err) {
		return err
	}
	if receipt.Remote != dest || receipt.Verified == nil {
		return fmt.Errorf("invalid project receipt for %s", dest)
	}
	var missing []string
	for dir, kind := range dirs {
		if kind == pcsMissing {
			missing = append(missing, dir)
		}
	}
	sort.Strings(missing) // Parents are prefixes of their children.
	for _, dir := range missing {
		if err = e.command(ctx, "mkdir", dir); err != nil {
			return err
		}
		kind, err := e.pcsStat(ctx, dir)
		if err != nil {
			return err
		}
		if kind != pcsDirectory {
			return fmt.Errorf("Baidu directory creation failed: %s", dir)
		}
	}
	for _, f := range files {
		if already[f.Path] {
			fmt.Fprintf(out, "Already verified: %s\n", f.Path)
		} else {
			remote := path.Join(dest, filepath.ToSlash(f.Path))
			if err = e.sendProjectFile(ctx, root, remote, f); err != nil {
				return fmt.Errorf("%s: %w", f.Path, err)
			}
			fmt.Fprintf(out, "Uploaded and verified: %s\n", f.Path)
		}
		receipt.Verified[f.Path] = f.SHA256
		receipt.Updated = time.Now().UTC()
		if err = writeJSON(receiptPath, receipt); err != nil {
			return err
		}
	}
	return nil
}

type projectReceipt struct {
	Remote   string            `json:"remote"`
	Verified map[string]string `json:"verified"`
	Updated  time.Time         `json:"updated"`
}

func (e *Engine) projectRemoteDir(ctx context.Context, dir string, known map[string]pcsKind) (pcsKind, error) {
	if kind, ok := known[dir]; ok {
		return kind, nil
	}
	parent, err := e.projectRemoteDir(ctx, path.Dir(dir), known)
	if err != nil {
		return pcsMissing, err
	}
	kind := pcsMissing
	if parent == pcsDirectory {
		kind, err = e.pcsStat(ctx, dir)
		if err != nil {
			return pcsMissing, err
		}
		if kind == pcsFile {
			return pcsMissing, fmt.Errorf("remote directory path is a file: %s", dir)
		}
	}
	known[dir] = kind
	return kind, nil
}

func (e *Engine) verifyProjectRemote(ctx context.Context, remote string, f File) error {
	timeout, cancel := context.WithTimeout(ctx, time.Duration(e.Config.UploadTimeoutMinutes)*time.Minute)
	defer cancel()
	return e.verifyRemote(timeout, remote, f)
}

func (e *Engine) sendProjectFile(ctx context.Context, root, remote string, f File) error {
	timeout, cancel := context.WithTimeout(ctx, time.Duration(e.Config.UploadTimeoutMinutes)*time.Minute)
	defer cancel()
	scratch := filepath.Join(e.Config.Target, ".verify")
	if err := noSymlink(scratch); err != nil {
		return err
	}
	if err := os.MkdirAll(scratch, 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(scratch, "project-upload-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	local := filepath.Join(dir, filepath.Base(f.Path))
	source := filepath.Join(root, f.Path)
	if err = noSymlink(source); err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is no longer a regular file")
	}
	if err = verifiedCopy(timeout, source, local, f); err != nil {
		return err
	}
	if err = e.command(timeout, "upload", "--policy", "skip", "--norapid", local, path.Dir(remote)); err != nil {
		return err
	}
	return e.verifyRemote(timeout, remote, f)
}
