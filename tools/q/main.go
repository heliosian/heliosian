package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"heliosian/internal/db"
	"heliosian/internal/env"
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

func render(w io.Writer, table string, ids []string, rows map[string]map[string]string) {
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
	fmt.Fprintf(w, "%s: %d\n", table, len(ids))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(columns, "\t"))
	for _, id := range ids {
		cells := []string{}
		for _, c := range columns {
			cells = append(cells, cell(rows[id][c]))
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	tw.Flush()
}

func show(w io.Writer, a qclient.Answer) {
	fmt.Fprintln(w, a.Query)
	fmt.Fprintln(w)
	listed := ""
	if len(a.Result) > 0 {
		listed, _ = db.TableOf(a.Result[0])
		render(w, listed, a.Result, a.Resources[listed])
	} else {
		fmt.Fprintln(w, "no rows")
	}
	for _, table := range slices.Sorted(maps.Keys(a.Resources)) {
		if table == listed {
			continue
		}
		fmt.Fprintln(w)
		render(w, table, slices.Sorted(maps.Keys(a.Resources[table])), a.Resources[table])
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/q read <query>   (the query language, or a JSON tree; stdin when no argument)")
		fmt.Fprintln(os.Stderr, "       go run ./tools/q write <batch>  (a JSON batch; stdin when no argument)")
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	c := qclient.Client{Base: qclient.Production, Key: env.Required("IMPORT_KEY")}
	body := strings.TrimSpace(input(flag.Args()[1:]))
	switch flag.Arg(0) {
	case "read":
		var a qclient.Answer
		var err error
		if strings.HasPrefix(body, "{") {
			a, err = c.Query(json.RawMessage(body))
		} else {
			a, err = c.QueryText(body)
		}
		if err != nil {
			logging.Fatal("query", "error", err)
		}
		show(os.Stdout, a)
	case "write":
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
