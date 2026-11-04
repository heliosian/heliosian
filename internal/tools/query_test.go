package tools

import (
	"encoding/json"
	"fmt"
	"testing"

	"heliosian/internal/db"
	"heliosian/internal/store"
)

func TestQueriesKeepIncludedRowsOutsideTheirResults(t *testing.T) {
	set, s := sample(t)
	var child store.Row
	for _, row := range s.Model().Table("GROUP").All() {
		if row["kind"] == "crew" {
			child = row
			break
		}
	}
	if child == nil {
		t.Fatal("no sample crew")
	}
	for _, name := range []string{"helios_query", "helios_query_json"} {
		input := map[string]any{"query": fmt.Sprintf(`(from GROUP (where (= id %q)) (include parent))`, child["id"])}
		if name == "helios_query_json" {
			input["query"] = scan("GROUP", where(eq("id", child["id"])), tree{"include", "parent"})
		}
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		answer, err := run(t, set, s, name, string(raw))
		if err != nil {
			t.Fatal(err)
		}
		var out rows
		if err := json.Unmarshal([]byte(answer.Text), &out); err != nil {
			t.Fatal(err)
		}
		if out.Count != 1 || len(out.Rows) != 1 || out.Rows[0]["id"] != child["id"] {
			t.Fatalf("%s: unexpected results: %s", name, answer.Text)
		}
		included := out.Included["GROUP"]
		if len(included) != 1 || included[child["parent"]]["id"] != child["parent"] {
			t.Fatalf("%s: unexpected includes: %s", name, answer.Text)
		}
	}
}

func BenchmarkShapeQuery(b *testing.B) {
	for _, n := range []int{1000, 10000, 50000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			res := db.Result{Table: "DOCUMENT", Resources: map[string]map[string]store.Row{"DOCUMENT": {}}}
			for i := range n {
				id := fmt.Sprintf("doc%011d", i)
				res.IDs = append(res.IDs, id)
				res.Resources[res.Table][id] = store.Row{"id": id}
			}
			c := &call{}
			b.ReportAllocs()
			for b.Loop() {
				out := c.shape(res, nil)
				if len(out.Rows) != n || len(out.Included) != 0 {
					b.Fatal("unexpected query answer")
				}
			}
		})
	}
}
