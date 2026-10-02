package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"slices"

	"heliosian/internal/env"
	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

func run(dir string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	dryRun := flag.Bool("dry-run", false, "print the batch the import would send, and send nothing")
	flag.Parse()

	c := client{qclient.Client{Base: qclient.Production, Key: env.Required("IMPORT_KEY")}}
	anthropicKey := env.Required("ANTHROPIC_API_KEY")
	exporter := env.Required("VCEXPORT")
	website := env.Required("WEBEXPORT")
	out, err := os.MkdirTemp("", "vcexport")
	if err != nil {
		logging.Fatal("create the output directory", "error", err)
	}
	err = importDirectory(c, exporter, website, out, *dryRun)
	if rmErr := os.RemoveAll(out); rmErr != nil {
		logging.Fatal("remove the export", "dir", out, "error", rmErr)
	}
	if err != nil {
		logging.Fatal("directory import", "error", err)
	}
	if err := importCalendar(c, anthropicKey, *dryRun); err != nil {
		logging.Fatal("calendar import", "error", err)
	}
}

func importDirectory(c client, exporter, website, out string, dryRun bool) error {
	slog.Info("exporting from veracross", "into", out)
	if err := run(exporter, "go", "run", ".", "--out", out); err != nil {
		return fmt.Errorf("veracross export: %w", err)
	}
	slog.Info("exporting the school website's staff page", "into", out)
	if err := run(website, "go", "run", ".", "--out", out); err != nil {
		return fmt.Errorf("website export: %w", err)
	}
	x, err := readExport(out)
	if err != nil {
		return fmt.Errorf("read the export: %w", err)
	}
	p, err := planned(c, x)
	if err != nil {
		return fmt.Errorf("plan the import: %w", err)
	}
	attrs := []any{"writes", len(p.batch)}
	for _, k := range slices.Sorted(maps.Keys(p.counts)) {
		attrs = append(attrs, k, p.counts[k])
	}
	slog.Info("planned", attrs...)
	if dryRun {
		portraits, err := p.portraits()
		if err != nil {
			return fmt.Errorf("read the photos: %w", err)
		}
		slog.Info("photos to add", "photos", len(portraits))
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"batch": p.batch})
	}
	added, err := apply(c, x, p)
	if err != nil {
		return err
	}
	slog.Info("photos added", "photos", added)
	return nil
}

func planned(c client, x *export) (*planner, error) {
	st, err := c.read()
	if err != nil {
		return nil, err
	}
	return plan(x, st)
}

func apply(c client, x *export, p *planner) (int, error) {
	if len(p.batch) > 0 {
		written, err := c.write(p.batch)
		if err != nil {
			return 0, err
		}
		slog.Info("written", "writes", len(written))
		if p, err = planned(c, x); err != nil {
			return 0, err
		}
		if len(p.batch) > 0 {
			return 0, fmt.Errorf("after writing, a fresh plan still has %d writes", len(p.batch))
		}
	}
	portraits, err := p.portraits()
	if err != nil {
		return 0, err
	}
	for _, photo := range portraits {
		if err := c.addPhoto(photo); err != nil {
			return 0, err
		}
	}
	return len(portraits), nil
}
