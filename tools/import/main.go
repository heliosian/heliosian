package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"slices"

	"heliosian/internal/env"
	"heliosian/internal/logging"
)

const server = "https://who.heliosian.com/api/q"

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

	c := client{url: server, key: env.Required("IMPORT_KEY")}
	exporter := env.Required("VCEXPORT")
	website := env.Required("WEBEXPORT")
	out, err := os.MkdirTemp("", "vcexport")
	if err != nil {
		logging.Fatal("create the output directory", "error", err)
	}
	slog.Info("exporting from veracross", "into", out)
	if err := run(exporter, "go", "run", ".", "--out", out); err != nil {
		logging.Fatal("veracross export", "error", err)
	}
	slog.Info("exporting the school website's staff page", "into", out)
	if err := run(website, "go", "run", ".", "--out", out); err != nil {
		logging.Fatal("website export", "error", err)
	}

	x, err := readExport(out)
	if err != nil {
		logging.Fatal("read the export", "error", err)
	}
	st, err := c.read()
	if err != nil {
		logging.Fatal("read the data model", "error", err)
	}
	batch, counts, err := plan(x, st)
	if err != nil {
		logging.Fatal("plan the import", "error", err)
	}
	attrs := []any{"writes", len(batch)}
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		attrs = append(attrs, k, counts[k])
	}
	slog.Info("planned", attrs...)
	if *dryRun {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"batch": batch}); err != nil {
			logging.Fatal("print the batch", "error", err)
		}
		return
	}
	if len(batch) == 0 {
		return
	}
	written, err := c.write(batch)
	if err != nil {
		logging.Fatal("write the batch", "error", err)
	}
	slog.Info("written", "writes", len(written))
}
