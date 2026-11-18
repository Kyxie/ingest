package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kyxie/ingest/internal/ingest"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ingest stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	cfg := fs.String("config", "/etc/ingest/config.json", "configuration file")
	once := fs.Bool("once", false, "run one cycle")
	batch := fs.String("batch", "", "batch ID to release")
	device := fs.String("device", "", "device key for a project upload")
	project := fs.String("project", "", "project directory name")
	apply := fs.Bool("apply", false, "upload after previewing the project")
	merge := fs.Bool("merge", false, "allow an existing remote project directory")
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: ingest scan|upload|project-upload|publish-nikon|release|prune --config PATH")
	}
	mode := os.Args[1]
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	c, err := ingest.Load(*cfg)
	if err != nil {
		return err
	}
	e, err := ingest.New(c)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch mode {
	case "project-upload":
		return e.UploadProject(ctx, *device, *project, *apply, *merge, os.Stdout)
	case "publish-nikon":
		return e.PublishNikon(ctx, *batch)
	case "release":
		return e.Release(*batch)
	case "prune":
		return e.Prune(ctx)
	case "scan", "upload":
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
	for {
		var err error
		if mode == "scan" {
			err = e.Scan(ctx)
			if err == nil && c.AutoPrune {
				err = e.Prune(ctx)
			}
		} else {
			err = e.Upload(ctx)
		}
		if *once {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			slog.Error("cycle failed; will retry", "mode", mode, "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(c.PollSeconds) * time.Second):
		}
	}
}
