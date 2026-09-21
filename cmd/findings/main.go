// Command findings lists the security audit's findings.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const dir = "security-audit/findings"

var statuses = map[string]bool{"open": true, "fixed": true, "wontfix": true, "invalid": true}

type finding struct {
	path        string
	description string
	status      string
	text        string
	created     string
	updated     string
}

func main() {
	status := flag.String("status", "", "comma-separated statuses to keep: open, fixed, wontfix, invalid")
	text := flag.String("text", "", "keep findings with these words anywhere in the file, case-insensitive")
	since := flag.String("since", "", "keep findings changed on or after this day, as 2006-01-02")
	flag.Parse()
	keep := map[string]bool{}
	if *status != "" {
		for _, s := range strings.Split(*status, ",") {
			if !statuses[s] {
				log.Fatalf("[ERROR] unknown status %q", s)
			}
			keep[s] = true
		}
	}
	if *since != "" {
		if _, err := time.Parse(time.DateOnly, *since); err != nil {
			log.Fatalf("[ERROR] --since: %v", err)
		}
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		log.Fatalf("[ERROR] list %s: %v", dir, err)
	}
	shown := 0
	for _, path := range paths {
		f := load(path)
		if len(keep) > 0 && !keep[f.status] {
			continue
		}
		if !strings.Contains(strings.ToLower(f.text), strings.ToLower(*text)) {
			continue
		}
		if *since != "" && f.updated != "" && f.updated < *since {
			continue
		}
		shown++
		if f.created == "" {
			fmt.Printf("%-8s %s  uncommitted\n", f.status, f.path)
		} else {
			fmt.Printf("%-8s %s  created %s  updated %s\n", f.status, f.path, f.created, f.updated)
		}
		fmt.Printf("  %s\n", f.description)
	}
	log.Printf("%d of %d findings", shown, len(paths))
}

func load(path string) finding {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", path, err)
	}
	f := finding{path: path, text: string(raw)}
	lines := strings.Split(f.text, "\n")
	if len(lines) < 3 {
		log.Fatalf("[ERROR] %s: want a Description line, a Status line, a --- line, then the details", path)
	}
	f.description = field(path, lines[0], "Description")
	f.status = field(path, lines[1], "Status")
	if !statuses[f.status] {
		log.Fatalf("[ERROR] %s: unknown status %q", path, f.status)
	}
	if lines[2] != "---" {
		log.Fatalf("[ERROR] %s: line 3 must be ---, got %q", path, lines[2])
	}
	out, err := exec.Command("git", "log", "--follow", "--format=%as", "--", path).Output()
	if err != nil {
		log.Fatalf("[ERROR] git log %s: %v", path, err)
	}
	dates := strings.Fields(string(out))
	if len(dates) > 0 {
		f.updated = dates[0]
		f.created = dates[len(dates)-1]
	}
	return f
}

func field(path, line, name string) string {
	value, ok := strings.CutPrefix(line, name+": ")
	if !ok || strings.TrimSpace(value) == "" {
		log.Fatalf("[ERROR] %s: want a line starting %q, got %q", path, name+": ", line)
	}
	return strings.TrimSpace(value)
}
