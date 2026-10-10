package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"heliosian/internal/db"
	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

const cellWidth = 48

func input(args []string) string {
	if len(args) > 0 {
		return strings.Join(args, " ")
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		logging.Fatal("read stdin", "error", err)
	}
	return string(raw)
}

func cell(v string) string {
	v = strings.ReplaceAll(strings.TrimSpace(v), "\n", "⏎")
	if utf8.RuneCountInString(v) > cellWidth {
		return string([]rune(v)[:cellWidth-1]) + "…"
	}
	return v
}

func render(w io.Writer, table string, ids []string, rows map[string]map[string]string, chosen []string) {
	t, ok := db.Lookup(table)
	if !ok {
		logging.Fatal("the answer names a table the schema lacks", "table", table)
	}
	columns := []string{}
	for _, c := range t.Columns {
		if slices.ContainsFunc(ids, func(id string) bool { return rows[id][c.Name] != "" }) {
			columns = append(columns, c.Name)
		}
	}
	format := cell
	if len(chosen) > 0 {
		for _, name := range chosen {
			if !slices.ContainsFunc(t.Columns, func(c db.Column) bool { return c.Name == name }) {
				logging.Fatal("the listed table has no such column", "table", table, "column", name)
			}
		}
		columns = chosen
		format = func(v string) string { return strings.ReplaceAll(strings.TrimSpace(v), "\n", "⏎") }
	}
	fmt.Fprintf(w, "%s: %d\n", table, len(ids))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(columns, "\t"))
	for _, id := range ids {
		cells := []string{}
		for _, c := range columns {
			cells = append(cells, format(rows[id][c]))
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	tw.Flush()
}

func show(w io.Writer, a qclient.Answer, columns []string) {
	fmt.Fprintln(w, a.Query)
	fmt.Fprintln(w)
	listed := ""
	if len(a.Result) > 0 {
		listed, _ = db.TableOf(a.Result[0])
		render(w, listed, a.Result, a.Resources[listed], columns)
	} else {
		fmt.Fprintln(w, "no rows")
	}
	for _, table := range slices.Sorted(maps.Keys(a.Resources)) {
		if table == listed {
			continue
		}
		fmt.Fprintln(w)
		render(w, table, slices.Sorted(maps.Keys(a.Resources[table])), a.Resources[table], nil)
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/q [--spoof email] read [--columns a,b] [--trace] <query>   (the query language, or a JSON tree; stdin when no argument)")
		fmt.Fprintln(os.Stderr, "       go run ./tools/q [--spoof email] write <batch>                   (a JSON batch; stdin when no argument)")
	}
	spoof := flag.String("spoof", "", "run as the person with this address, as a super admin's Spoof Mode does")
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	c, err := qclient.SignedIn(qclient.ImportMode, *spoof)
	if err != nil {
		logging.Fatal("sign in", "error", err)
	}
	switch flag.Arg(0) {
	case "read":
		fs := flag.NewFlagSet("read", flag.ExitOnError)
		chosen := fs.String("columns", "", "comma-separated columns of the listed table to print, uncut, in place of every filled one cut short")
		traced := fs.Bool("trace", false, "print the round trip's time and the server's trace of the query after the rows")
		fs.Parse(flag.Args()[1:])
		columns := []string{}
		if *chosen != "" {
			columns = strings.Split(*chosen, ",")
		}
		body := strings.TrimSpace(input(fs.Args()))
		var a qclient.Answer
		var err error
		start := time.Now()
		if strings.HasPrefix(body, "{") {
			a, err = c.Query(json.RawMessage(body))
		} else {
			a, err = c.QueryText(body)
		}
		took := time.Since(start)
		if err != nil {
			logging.Fatal("query", "error", err)
		}
		show(os.Stdout, a, columns)
		if *traced {
			var tree bytes.Buffer
			if err := json.Indent(&tree, []byte(a.Trace), "", "  "); err != nil {
				logging.Fatal("read the trace", "error", err, "trace", a.Trace)
			}
			fmt.Printf("\nround trip %s\n%s\n", took.Round(time.Millisecond), tree.String())
		}
	case "write":
		body := strings.TrimSpace(input(flag.Args()[1:]))
		ids, err := c.Write(json.RawMessage(body))
		if err != nil {
			logging.Fatal("write", "error", err)
		}
		for _, id := range ids {
			fmt.Println(id)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}
