// Command findings lists the security audit's findings.
package main

import (
	"cmp"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const dir = "security-audit/findings"

var statuses = []string{"open", "fixed", "wontfix", "invalid"}

var severities = []string{"critical", "high", "medium", "low"}

var severityColor = map[string]string{"critical": "1;97;41", "high": "1;31", "medium": "33", "low": "36"}

var statusColor = map[string]string{"open": "1", "fixed": "32", "wontfix": "35", "invalid": "90"}

type finding struct {
	path        string
	description string
	status      string
	severity    string
	text        string
	created     string
	updated     string
}

func main() {
	status := flag.String("status", "", "comma-separated statuses to keep: open, fixed, wontfix, invalid")
	text := flag.String("text", "", "keep findings with these words anywhere in the file, case-insensitive")
	since := flag.String("since", "", "keep findings changed on or after this day, as 2006-01-02")
	stat := flag.Bool("stat", false, "print a grid of counts by severity and status instead of the list")
	first := flag.Bool("first", false, "keep only the first open finding, by file name, at the highest severity any open finding has")
	flag.Parse()
	keep := map[string]bool{}
	if *status != "" {
		for _, s := range strings.Split(*status, ",") {
			if !slices.Contains(statuses, s) {
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
	shown := []finding{}
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
		shown = append(shown, f)
	}
	if *first {
		shown = firstOpen(shown)
	}
	if *stat {
		grid(shown)
	} else {
		list(shown)
	}
	log.Printf("%d of %d findings", len(shown), len(paths))
}

func firstOpen(found []finding) []finding {
	open := slices.DeleteFunc(slices.Clone(found), func(f finding) bool { return f.status != "open" })
	if len(open) == 0 {
		return open
	}
	slices.SortStableFunc(open, func(a, b finding) int {
		return cmp.Or(cmp.Compare(slices.Index(severities, a.severity), slices.Index(severities, b.severity)), cmp.Compare(a.path, b.path))
	})
	return open[:1]
}

func list(found []finding) {
	for _, f := range found {
		if f.created == "" {
			fmt.Printf("%-8s %-8s %s  uncommitted\n", f.status, f.severity, f.path)
		} else {
			fmt.Printf("%-8s %-8s %s  created %s  updated %s\n", f.status, f.severity, f.path, f.created, f.updated)
		}
		fmt.Printf("  %s\n", f.description)
	}
}

func grid(found []finding) {
	counts := map[string]map[string]int{}
	for _, severity := range severities {
		counts[severity] = map[string]int{}
	}
	for _, f := range found {
		counts[f.severity][f.status]++
	}
	fmt.Print(left("", "", 10))
	for _, status := range statuses {
		fmt.Print(right(statusColor[status], status, 9))
	}
	fmt.Println(right("1", "total", 9))
	columns := map[string]int{}
	for _, severity := range severities {
		fmt.Print(left(severityColor[severity], severity, 10))
		row := 0
		for _, status := range statuses {
			n := counts[severity][status]
			row += n
			columns[status] += n
			color := statusColor[status]
			if status == "open" {
				color = severityColor[severity]
			}
			fmt.Print(count(color, n, 9))
		}
		fmt.Println(count("1", row, 9))
	}
	fmt.Print(left("1", "total", 10))
	for _, status := range statuses {
		fmt.Print(count("1", columns[status], 9))
	}
	fmt.Println(count("1", len(found), 9))
}

func count(color string, n, width int) string {
	if n == 0 {
		return right("90", "·", width)
	}
	return right(color, strconv.Itoa(n), width)
}

func paint(color, s string) string {
	if color == "" || s == "" {
		return s
	}
	return "\x1b[" + color + "m" + s + "\x1b[0m"
}

func left(color, s string, width int) string {
	return paint(color, s) + strings.Repeat(" ", max(width-len([]rune(s)), 0))
}

func right(color, s string, width int) string {
	return strings.Repeat(" ", max(width-len([]rune(s)), 0)) + paint(color, s)
}

func load(path string) finding {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", path, err)
	}
	f := finding{path: path, text: string(raw)}
	lines := strings.Split(f.text, "\n")
	if len(lines) < 4 {
		log.Fatalf("[ERROR] %s: want a Description line, a Status line, a Severity line, a --- line, then the details", path)
	}
	f.description = field(path, lines[0], "Description")
	f.status = field(path, lines[1], "Status")
	if !slices.Contains(statuses, f.status) {
		log.Fatalf("[ERROR] %s: unknown status %q", path, f.status)
	}
	f.severity = field(path, lines[2], "Severity")
	if !slices.Contains(severities, f.severity) {
		log.Fatalf("[ERROR] %s: unknown severity %q", path, f.severity)
	}
	if lines[3] != "---" {
		log.Fatalf("[ERROR] %s: line 4 must be ---, got %q", path, lines[3])
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
