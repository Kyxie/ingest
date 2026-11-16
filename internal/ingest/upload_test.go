package ingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadRequiresRemoteReadback(t *testing.T) {
	for _, ok := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong_remote", true: "verified"}[ok], func(t *testing.T) {
			c, src := fixture(t)
			put(t, filepath.Join(src, "A.JPG"), "photo")
			script := filepath.Join(t.TempDir(), "pcs")
			payload := "wrong"
			if ok {
				payload = "photo"
			}
			body := "#!/bin/sh\ncase \"$1\" in\nupload) exit 0;;\ndownload) printf '" + payload + "' > \"$3/A.JPG\";;\nesac\n"
			if err := os.WriteFile(script, []byte(body), 0755); err != nil {
				t.Fatal(err)
			}
			c.PCS = script
			e, _ := New(c)
			if err := e.Scan(context.Background()); err != nil {
				t.Fatal(err)
			}
			err := e.Upload(context.Background())
			if (err == nil) != ok {
				t.Fatalf("upload result %v", err)
			}
			bs, _ := e.Batches()
			var r Receipt
			if err := readJSON(e.receipt(bs[0].ID), &r); err != nil {
				t.Fatal(err)
			}
			if (len(r.Verified) == 1) != ok {
				t.Fatal("incorrect verification receipt")
			}
		})
	}
}
func TestNoVerificationCannotAuthorizeRetention(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.JPG"), "photo")
	script := filepath.Join(t.TempDir(), "pcs")
	os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755)
	c.PCS = script
	c.VerifyDownload = false
	e, _ := New(c)
	e.Scan(context.Background())
	if err := e.Upload(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ := e.Batches()
	var r Receipt
	readJSON(e.receipt(bs[0].ID), &r)
	if len(r.Verified) != 0 || len(r.Uploaded) != 1 {
		t.Fatal("unverified upload marked safe")
	}
}

func TestProxyOnlyBatchGetsCompletionReceipt(t *testing.T) {
	c, src := fixture(t)
	put(t, filepath.Join(src, "A.LRF"), "proxy")
	e, _ := New(c)
	if err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Upload(context.Background()); err != nil {
		t.Fatal(err)
	}
	bs, _ := e.Batches()
	var r Receipt
	if err := readJSON(e.receipt(bs[0].ID), &r); err != nil {
		t.Fatal(err)
	}
}
