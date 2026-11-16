package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) (Config, string) {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "cards", "nikon", "DCIM", "100NIKON")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	return Config{Sources: filepath.Join(root, "cards"), Target: filepath.Join(root, "media"), State: filepath.Join(root, "state"), StableSeconds: 0, PollSeconds: 1, RetentionDays: 90, Remote: "/ingest", PCS: "BaiduPCS-Go", VerifyDownload: true}, src
}
func put(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestVerifiedImportAndSelection(t *testing.T) {
	c, src := fixture(t)
	for n, s := range map[string]string{"DSC_0001.NEF": "raw", "DSC_0001.JPG": "jpeg", "DSC_0002.JPG": "onlyjpeg", "DJI_0001.MP4": "video", "DJI_0001.LRF": "proxy", "DJI_0001.WAV": "audio"} {
		put(t, filepath.Join(src, n), s)
	}
	e, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, err := e.Batches()
	if err != nil || len(bs) != 1 {
		t.Fatalf("batches %v %v", bs, err)
	}
	b := bs[0]
	for _, f := range b.Files {
		p := e.original(b.ID, f.Path)
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			t.Fatalf("copy %s %v", p, err)
		}
		if f.Archive == (filepath.Ext(f.Path) == ".LRF" || filepath.Base(f.Path) == "DSC_0001.JPG") {
			t.Errorf("archive %s=%v", f.Path, f.Archive)
		}
		if f.Share {
			a, _ := os.Stat(p)
			s, err := os.Stat(e.shared(b.ID, f.Path))
			if err != nil || !os.SameFile(a, s) {
				t.Errorf("share must be hardlinked %s: %v", f.Path, err)
			}
		}
	}
	if err = e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ = e.Batches()
	if len(bs) != 1 {
		t.Fatal("duplicate batch")
	}
	// A new insertion with one added file only imports that new content.
	put(t, filepath.Join(src, "DSC_0003.JPG"), "new")
	if err = e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ = e.Batches()
	if len(bs) != 2 {
		t.Fatal("missing incremental batch")
	}
	total := 0
	for _, b := range bs {
		total += len(b.Files)
	}
	if total != 7 {
		t.Fatalf("reimported old files: %d", total)
	}
}
func TestSourceSymlinkRejected(t *testing.T) {
	c, src := fixture(t)
	secret := filepath.Join(t.TempDir(), "secret")
	put(t, secret, "private")
	if err := os.Symlink(secret, filepath.Join(src, "DSC.JPG")); err != nil {
		t.Fatal(err)
	}
	e, _ := New(c)
	if err := e.Scan(context.Background()); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestOverlappingPathsRejected(t *testing.T) {
	c, _ := fixture(t)
	c.Target = filepath.Join(c.Sources, "output")
	if _, err := New(c); err == nil {
		t.Fatal("recursive source accepted")
	}
}
func TestInterruptedCopyRecoversAndDoesNotOverwrite(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.JPG"), "content")
	e, _ := New(c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.Scan(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ := e.Batches()
	put(t, e.original(bs[0].ID, bs[0].Files[0].Path), "corrupt")
	e, _ = New(c)
	if err := e.Scan(context.Background()); err == nil {
		t.Fatal("ignored corrupted destination")
	}
}
func TestRetentionNeedsReceiptAndRelease(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.JPG"), "photo")
	e, _ := New(c)
	e.Scan(context.Background())
	bs, _ := e.Batches()
	b := bs[0]
	b.Created = time.Now().Add(-100 * 24 * time.Hour)
	writeJSON(e.manifest(b.ID), b)
	if err := e.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.original(b.ID, b.Files[0].Path)); err != nil {
		t.Fatal("deleted unarchived file")
	}
	r := Receipt{Verified: map[string]string{b.Files[0].Path: b.Files[0].SHA256}}
	writeJSON(e.receipt(b.ID), r)
	e.Prune(context.Background())
	if _, err := os.Stat(e.original(b.ID, b.Files[0].Path)); err != nil {
		t.Fatal("deleted active share")
	}
	e.Release(b.ID)
	if err := e.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.original(b.ID, b.Files[0].Path)); !os.IsNotExist(err) {
		t.Fatal("not pruned")
	}
}

func TestFutureCameraClockDoesNotPreventImport(t *testing.T) {
	c, src := fixture(t)
	c.StableSeconds = 10
	p := filepath.Join(src, "A.JPG")
	put(t, p, "photo")
	future := time.Now().AddDate(10, 0, 0)
	os.Chtimes(p, future, future)
	e, _ := New(c)
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for k, v := range e.observed {
		v.Since = time.Now().Add(-time.Minute)
		e.observed[k] = v
	}
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ := e.Batches()
	if len(bs) != 1 {
		t.Fatal("future camera date blocked import")
	}
}
func TestUnrelatedOldMissingFileDoesNotBlockNewCard(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.JPG"), "old")
	e, _ := New(c)
	e.Scan(context.Background())
	bs, _ := e.Batches()
	os.Remove(e.original(bs[0].ID, bs[0].Files[0].Path))
	os.Remove(filepath.Join(src, "A.JPG"))
	put(t, filepath.Join(src, "B.JPG"), "new")
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ = e.Batches()
	if len(bs) != 2 {
		t.Fatal("old missing batch blocked new import")
	}
}

func TestIncompleteManifestResumesAfterRestart(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.JPG"), "photo")
	put(t, filepath.Join(src, "B.MP4"), "video")
	e, _ := New(c)
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ := e.Batches()
	b := bs[0]
	b.Complete = false
	if err := writeJSON(e.manifest(b.ID), b); err != nil {
		t.Fatal(err)
	}
	for _, f := range b.Files {
		os.Remove(e.shared(b.ID, f.Path))
	}
	os.Remove(e.original(b.ID, b.Files[1].Path))
	e, _ = New(c)
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ = e.Batches()
	if len(bs) != 1 || !bs[0].Complete {
		t.Fatal("did not resume original batch")
	}
	for _, f := range bs[0].Files {
		sum, err := digest(context.Background(), e.original(b.ID, f.Path))
		if err != nil || sum != f.SHA256 {
			t.Fatal("resumed file mismatch", err)
		}
	}
}
func TestManualFilesSurvivePrune(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.JPG"), "photo")
	e, _ := New(c)
	e.Scan(context.Background())
	bs, _ := e.Batches()
	b := bs[0]
	b.Created = time.Now().AddDate(-1, 0, 0)
	writeJSON(e.manifest(b.ID), b)
	writeJSON(e.receipt(b.ID), Receipt{Verified: map[string]string{b.Files[0].Path: b.Files[0].SHA256}})
	e.Release(b.ID)
	manual := filepath.Join(c.Target, "shared", "manual", "keep.txt")
	put(t, manual, "mine")
	if err := e.Prune(context.Background()); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(manual); err != nil || string(data) != "mine" {
		t.Fatal("manual content deleted")
	}
}

func TestJPEGInLaterBatchIsArchivedIndependently(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.NEF"), "raw")
	e, _ := New(c)
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(src, "A.JPG"), "jpeg")
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ := e.Batches()
	for _, b := range bs {
		for _, f := range b.Files {
			if filepath.Ext(f.Path) == ".JPG" && !f.Archive {
				t.Fatal("later JPEG depends on unverified older RAW batch")
			}
		}
	}
}
