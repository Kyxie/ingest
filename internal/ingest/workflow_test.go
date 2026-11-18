package ingest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNikonBrowseView(t *testing.T) {
	c, src := fixture(t)
	c.ImportTimezone = "America/Toronto"
	card := filepath.Dir(filepath.Dir(src))
	nikon := filepath.Join(card, "DCIM", "102NZ_FC")
	if err := os.Rename(src, nikon); err != nil {
		t.Fatal(err)
	}
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"DSC_3651.NEF": "raw", "DSC_3651.JPG": "jpeg", "NC_FLLST.DAT": "index"} {
		put(t, filepath.Join(nikon, name), content)
	}
	before := time.Now()
	if err = e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	batches, err := e.Batches()
	if err != nil || len(batches) != 1 {
		t.Fatalf("batches: %v %v", batches, err)
	}
	b := batches[0]
	loc, _ := time.LoadLocation(c.ImportTimezone)
	if b.View != before.In(loc).Format("20060102150405") && b.View != time.Now().In(loc).Format("20060102150405") {
		t.Fatalf("view %q is not a local timestamp", b.View)
	}
	viewRoot := filepath.Join(c.Target, "shared", "imports", b.View)
	for _, name := range []string{"raw/DSC_3651.NEF", "jpg/DSC_3651.JPG"} {
		if _, err = os.Stat(filepath.Join(viewRoot, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(filepath.Join(viewRoot, "NC_FLLST.DAT")); !os.IsNotExist(err) {
		t.Fatalf("housekeeping visible: %v", err)
	}
	if _, err = os.Stat(e.original(b.ID, "DCIM/102NZ_FC/NC_FLLST.DAT")); err != nil {
		t.Fatal(err)
	}
}

func TestPublishNikonKeepsLegacyPathsAndResolvesCollisions(t *testing.T) {
	c, src := fixture(t)
	ctx := context.Background()
	put(t, filepath.Join(src, "DSC_3651.JPG"), "first")
	put(t, filepath.Join(src, "DSC_3651.NEF"), "negative")
	second := filepath.Join(filepath.Dir(src), "101NIKON")
	if err := os.MkdirAll(second, 0755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(second, "DSC_3651.JPG"), "second")
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	batches, err := e.Batches()
	if err != nil || len(batches) != 1 {
		t.Fatalf("batches: %v %v", batches, err)
	}
	b := batches[0]
	// Simulate an old Nikon layout. The card and old HDD checkout are now independent.
	for i, file := range b.Files {
		if filepath.Dir(file.Path) != "DCIM/100NIKON" {
			continue
		}
		newPath := filepath.Join("DCIM/102NZ_FC", filepath.Base(file.Path))
		for _, top := range []string{e.original(b.ID, "."), e.shared(b.ID, ".")} {
			if _, err := os.Stat(filepath.Join(top, file.Path)); os.IsNotExist(err) {
				continue
			}
			if err := os.MkdirAll(filepath.Join(top, filepath.Dir(newPath)), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(top, file.Path), filepath.Join(top, newPath)); err != nil {
				t.Fatal(err)
			}
		}
		b.Files[i].Path = newPath
	}
	if err := writeJSON(e.manifest(b.ID), b); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(c.Sources); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(e.shared(b.Created.In(time.UTC).Format("20060102150405"), "."), 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := e.PublishNikon(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := readJSON(e.manifest(b.ID), &b); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(b.View, "-02") {
		t.Fatalf("same-second collision was not resolved: %s", b.View)
	}
	for name, want := range map[string]string{
		e.shared(b.ID, "DCIM/102NZ_FC/DSC_3651.JPG"): "first",
		e.shared(b.View, "jpg/DSC_3651.JPG"):         "first",
		e.shared(b.View, "jpg/DSC_3651-02.JPG"):      "second",
		e.shared(b.View, "raw/DSC_3651.NEF"):         "negative",
	} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != want {
			t.Fatalf("%s: %q %v", name, data, err)
		}
	}
}

func TestProjectUploadPreservesNestedFilesAndProtectsRemote(t *testing.T) {
	c, _ := fixture(t)
	c.ProjectRemotes = map[string]string{"zfc": "/Camera Files/Nikon Zfc"}
	c.PCS = filepath.Join(filepath.Dir(c.State), "pcs")
	remote := filepath.Join(filepath.Dir(c.State), "netdisk")
	t.Setenv("TEST_REMOTE", remote)
	put(t, c.PCS, `#!/bin/sh
set -eu
case "$1" in
meta)
  printf '[0] - [%s] --------------\n' "$2"
  if [ -d "$TEST_REMOTE$2" ]; then
    printf '  类型  目录\n  目录路径  %s\n' "$2"
  elif [ -f "$TEST_REMOTE$2" ]; then
    printf '  类型  文件\n  文件路径  %s\n' "$2"
  else
    printf '获取文件/目录的元信息: 遇到错误, 远端服务器返回错误, 代码: 31066, 消息: 文件或目录不存在\n'
  fi ;;
mkdir) mkdir "$TEST_REMOTE$2" ;;
upload)
  test "$2" = --policy
  test "$3" = skip
  test ! -e "$TEST_REMOTE$6/${5##*/}"
  cp "$5" "$TEST_REMOTE$6/" ;;
download) cp "$TEST_REMOTE$5" "$3/" ;;
*) exit 1 ;;
esac
`)
	if err := os.Chmod(c.PCS, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(remote, "Camera Files", "Nikon Zfc"), 0755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(c.Target, "shared", "imports", "2026 青岛")
	if err := os.MkdirAll(filepath.Join(project, "raw"), 0755); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(project, "raw", "DSC_3651.NEF"), "negative")
	put(t, filepath.Join(project, "DSC_3651.JPG"), "photo")
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	destination := filepath.Join(remote, "Camera Files", "Nikon Zfc", "2026 青岛")
	if err = e.UploadProject(ctx, "zfc", "2026 青岛", false, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("preview modified remote: %v", err)
	}
	if err = e.UploadProject(ctx, "zfc", "2026 青岛", true, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"raw/DSC_3651.NEF": "negative", "DSC_3651.JPG": "photo"} {
		got, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil || string(got) != content {
			t.Fatalf("remote %s: %q %v", name, got, err)
		}
	}
	if err = e.UploadProject(ctx, "zfc", "2026 青岛", true, false, io.Discard); err == nil || !strings.Contains(err.Error(), "--merge") {
		t.Fatalf("retry without merge: %v", err)
	}
	if err = e.UploadProject(ctx, "zfc", "2026 青岛", true, true, io.Discard); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(project, "raw", "DSC_3651.NEF"), "changed")
	put(t, filepath.Join(project, "another.jpg"), "new")
	if err = e.UploadProject(ctx, "zfc", "2026 青岛", true, true, io.Discard); err == nil {
		t.Fatal("overlapping remote with different contents must fail")
	}
	if _, err = os.Stat(filepath.Join(destination, "another.jpg")); !os.IsNotExist(err) {
		t.Fatalf("conflict preflight wrote a file: %v", err)
	}
	if err := os.Symlink(filepath.Join(project, "DSC_3651.JPG"), filepath.Join(project, "link.jpg")); err != nil {
		t.Fatal(err)
	}
	if err = e.UploadProject(ctx, "zfc", "2026 青岛", false, false, io.Discard); err == nil {
		t.Fatal("preview accepted a symlink")
	}
}

func TestEditableImportsProtectOriginals(t *testing.T) {
	c, src := fixture(t)
	const oldName = "DSC_3651.JPG"
	put(t, filepath.Join(src, oldName), "old photo")
	legacy, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = legacy.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	batches, err := legacy.Batches()
	if err != nil || len(batches) != 1 {
		t.Fatalf("legacy batches: %v %v", batches, err)
	}
	b := batches[0]
	oldPath := filepath.Join("DCIM", "100NIKON", oldName)
	original := legacy.original(b.ID, oldPath)
	link := legacy.shared(b.ID, oldPath)
	a, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.Stat(link)
	if err != nil || !os.SameFile(a, s) {
		t.Fatal("expected an existing hardlinked import")
	}
	c.EditableImports = true
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	const newName = "DSC_3652.JPG"
	put(t, filepath.Join(src, newName), "new photo")
	if err = e.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.Target, "shared", ".imports-editable-v1")); err != nil {
		t.Fatal(err)
	}
	put(t, link, "edited old photo")
	if data, err := os.ReadFile(original); err != nil || string(data) != "old photo" {
		t.Fatalf("legacy original changed: %q %v", data, err)
	}
	batches, err = e.Batches()
	if err != nil || len(batches) != 2 {
		t.Fatalf("new batches: %v %v", batches, err)
	}
	for _, newest := range batches {
		if newest.ID == b.ID {
			continue
		}
		f := newest.Files[0]
		put(t, e.shared(newest.ID, f.Path), "edited new photo")
		if data, err := os.ReadFile(e.original(newest.ID, f.Path)); err != nil || string(data) != "new photo" {
			t.Fatalf("new original changed: %q %v", data, err)
		}
	}
	organized := e.shared("2026 青岛", oldName)
	if err := os.MkdirAll(filepath.Dir(organized), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(link, organized); err != nil {
		t.Fatal(err)
	}
	c.EditableImports = false
	restarted, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(link); !os.IsNotExist(err) {
		t.Fatalf("scan recreated the moved file: %v", err)
	}
	// Pruning a released old archive must leave the editable copy intact.
	b.Created = time.Now().Add(-120 * 24 * time.Hour)
	if err := writeJSON(e.manifest(b.ID), b); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(e.receipt(b.ID), Receipt{Verified: map[string]string{oldPath: b.Files[0].SHA256}}); err != nil {
		t.Fatal(err)
	}
	if err := e.Release(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("released original still present: %v", err)
	}
	if data, err := os.ReadFile(organized); err != nil || string(data) != "edited old photo" {
		t.Fatalf("editable copy was changed by prune: %q %v", data, err)
	}
}

func TestNikonArchiveOnlyKeepsCardPathsInManifest(t *testing.T) {
	c, src := fixture(t)
	c.ArchiveOnly = true
	nikon := filepath.Join(filepath.Dir(src), "102NZ_FC")
	if err := os.Rename(src, nikon); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"DSC_3651.NEF": "negative", "DSC_3651.JPG": "photo", "NC_FLLST.DAT": "index"}
	for name, data := range files {
		put(t, filepath.Join(nikon, name), data)
	}
	ctx := context.Background()
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	batches, err := e.Batches()
	if err != nil || len(batches) != 1 {
		t.Fatalf("batches: %v %v", batches, err)
	}
	b := batches[0]
	for card, stored := range map[string]string{
		"DSC_3651.NEF": "raw/DSC_3651.NEF",
		"DSC_3651.JPG": "jpg/DSC_3651.JPG",
		"NC_FLLST.DAT": "other/NC_FLLST.DAT",
	} {
		data, err := os.ReadFile(e.original(b.ID, stored))
		if err != nil || string(data) != files[card] {
			t.Fatalf("archive %s: %q %v", stored, data, err)
		}
	}
	for _, f := range b.Files {
		if filepath.Dir(f.Path) != "DCIM/102NZ_FC" || f.Stored == "" {
			t.Fatalf("card path or archive path missing: %+v", f)
		}
	}
	entries, err := os.ReadDir(e.shared(".", "."))
	if err != nil || len(entries) != 0 {
		t.Fatalf("auto-published share files: %v %v", entries, err)
	}
	e, err = New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	batches, err = e.Batches()
	if err != nil || len(batches) != 1 {
		t.Fatalf("duplicate batch after reinsertion: %v %v", batches, err)
	}
}

func TestClassifyOriginalsMigratesLegacyNikonBatch(t *testing.T) {
	c, src := fixture(t)
	nikon := filepath.Join(filepath.Dir(src), "102NZ_FC")
	if err := os.Rename(src, nikon); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(nikon, "DSC_3651.NEF"), "negative")
	put(t, filepath.Join(nikon, "DSC_3651.JPG"), "photo")
	ctx := context.Background()
	old, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	batches, err := old.Batches()
	if err != nil || len(batches) != 1 {
		t.Fatalf("batches: %v %v", batches, err)
	}
	b := batches[0]
	// Leave one target link as if a previous migration stopped before committing the manifest.
	prior := old.original(b.ID, "DCIM/102NZ_FC/DSC_3651.NEF")
	if err := shareLink(prior, old.original(b.ID, "raw/DSC_3651.NEF")); err != nil {
		t.Fatal(err)
	}
	c.ArchiveOnly, c.EditableImports = true, true
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := e.ClassifyOriginals(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = os.Stat(e.original(b.ID, "DCIM")); !os.IsNotExist(err) {
		t.Fatalf("old archive tree remains: %v", err)
	}
	for child, want := range map[string]string{"raw/DSC_3651.NEF": "negative", "jpg/DSC_3651.JPG": "photo"} {
		got, err := os.ReadFile(e.original(b.ID, child))
		if err != nil || string(got) != want {
			t.Fatalf("archive %s: %q %v", child, got, err)
		}
	}
	if err := e.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	batches, err = e.Batches()
	if err != nil || len(batches) != 1 {
		t.Fatalf("duplicate batch after migration: %v %v", batches, err)
	}
}
