package store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

type heldQueue struct {
	tasks []func()
}

func (q *heldQueue) Add(f func()) { q.tasks = append(q.tasks, f) }

func (q *heldQueue) run() {
	for len(q.tasks) > 0 {
		task := q.tasks[0]
		q.tasks = q.tasks[1:]
		task()
	}
}

type fixture struct {
	dir   *data.Dir
	store *Store[map[string]int]
}

func write(t *testing.T, root, tab, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "app", tab+".csv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func spec() Spec[map[string]int] {
	return Spec[map[string]int]{
		App: "app",
		Tabs: []Tab{
			{Name: "Things", Columns: []string{"Name", "Color", "Size"}, Key: []string{"Name"}, Cascade: func(before, after Row) []Op {
				if before == nil || after == nil || before["Name"] == after["Name"] {
					return nil
				}
				return []Op{Update("Uses", Row{"Thing": before["Name"]}, Row{"Thing": after["Name"]})}
			}},
			{Name: "Uses", Columns: []string{"Thing", "By"}, Key: []string{"Thing", "By"}},
			{Name: "Events", Columns: []string{"When", "What"}, Key: []string{"When"}, AppendOnly: true},
		},
		Build: func(tables Tables) (map[string]int, error) {
			for _, row := range tables["Things"] {
				if row["Color"] == "plaid" {
					return nil, errors.New("plaid is not a color")
				}
			}
			return map[string]int{"things": len(tables["Things"]), "uses": len(tables["Uses"])}, nil
		},
		Loaded: func(map[string]int, time.Duration) {},
	}
}

func newFixture(t *testing.T, queue Enqueuer) fixture {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "Things", "Name,Color,Size\nhat,red,small\nboot,black,large\n")
	write(t, root, "Uses", "Thing,By\nhat,ann\nhat,bo\nboot,ann\n")
	write(t, root, "Events", "When,What\n1,made\n")
	write(t, root, ChangeLogTab, strings.Join(ChangeLogColumns, ",")+"\n")
	dir := &data.Dir{Root: root}
	s, err := New(spec(), dir, dir, queue)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{dir: dir, store: s}
}

func (f fixture) rows(t *testing.T, tab string) []map[string]string {
	t.Helper()
	_, rows, err := f.dir.Table("app", tab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func (f fixture) log(t *testing.T) []string {
	t.Helper()
	out := []string{}
	for _, row := range f.rows(t, ChangeLogTab) {
		out = append(out, row["Actor"]+"|"+row["Real Actor"]+"|"+row["Action"]+"|"+row["Tab"]+"|"+row["Key"]+"|"+row["Column"]+"|"+row["Previous"])
	}
	return out
}

func equal(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s:\n got %q\nwant %q", what, got, want)
	}
}

func signedIn(email string) context.Context {
	var ctx context.Context
	auth.Fixed(email, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { ctx = r.Context() })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	return ctx
}

func TestCommitKeepsPreviousValuesOnly(t *testing.T) {
	f := newFixture(t, syncQueue{})
	ctx := signedIn("admin@example.org")
	err := f.store.Commit(ctx, "ann@example.org",
		Insert("Things", Row{"Name": "cap", "Color": "blue"}),
		Update("Things", Row{"Name": "boot"}, Row{"Color": "brown", "Size": ""}),
		Delete("Uses", Row{"Thing": "boot"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if m := f.store.Model(); m["things"] != 3 || m["uses"] != 2 {
		t.Fatalf("model %v", m)
	}
	equal(t, "change log", f.log(t), []string{
		"ann@example.org|admin@example.org|insert|Things|Name=cap||",
		"ann@example.org|admin@example.org|set|Things|Name=boot|Color|black",
		"ann@example.org|admin@example.org|set|Things|Name=boot|Size|large",
		"ann@example.org|admin@example.org|delete|Uses|Thing=boot; By=ann|By|ann",
		"ann@example.org|admin@example.org|delete|Uses|Thing=boot; By=ann|Thing|boot",
	})
	things := f.rows(t, "Things")
	if len(things) != 3 || things[1]["Color"] != "brown" || things[1]["Size"] != "" || things[2]["Name"] != "cap" {
		t.Fatalf("sheet %v", things)
	}
}

func TestSetInsertsAndUpdateDoesNot(t *testing.T) {
	f := newFixture(t, syncQueue{})
	if err := f.store.Commit(context.Background(), "job", Update("Things", Row{"Name": "sock"}, Row{"Color": "grey"})); err != nil {
		t.Fatal(err)
	}
	if len(f.rows(t, "Things")) != 2 || len(f.log(t)) != 0 {
		t.Fatal("an update matching nothing wrote something")
	}
	if err := f.store.Commit(context.Background(), "job", Set("Things", Row{"Name": "sock"}, Row{"Color": "grey"})); err != nil {
		t.Fatal(err)
	}
	if len(f.rows(t, "Things")) != 3 || f.store.Count("Things", Row{"Name": "SOCK"}) != 1 {
		t.Fatal("a set matching nothing did not insert")
	}
	equal(t, "change log", f.log(t), []string{"job||insert|Things|Name=sock||"})
}

func TestAppendOnlyIsWrittenNotLogged(t *testing.T) {
	f := newFixture(t, syncQueue{})
	if err := f.store.Commit(context.Background(), "job", Insert("Events", Row{"When": "2", "What": "sent"})); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Commit(context.Background(), "job", Delete("Events", Row{"When": "1"})); err != nil {
		t.Fatal(err)
	}
	if events := f.rows(t, "Events"); len(events) != 1 || events[0]["What"] != "sent" {
		t.Fatalf("sheet %v", events)
	}
	if len(f.log(t)) != 0 {
		t.Fatalf("an append-only tab was logged: %v", f.log(t))
	}
	for _, op := range []Op{Update("Events", Row{"When": "2"}, Row{"What": "bounced"}), Set("Events", Row{"When": "3"}, Row{"What": "sent"})} {
		if err := f.store.Commit(context.Background(), "job", op); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("an edit of an append-only tab was taken: %v", err)
		}
	}
}

type lineQueue chan func()

func newLineQueue(t *testing.T) lineQueue {
	q := make(lineQueue, 16)
	go func() {
		for f := range q {
			f()
		}
	}()
	t.Cleanup(func() { close(q) })
	return q
}

func (q lineQueue) Add(f func()) { q <- f }

func TestCommitAndWaitWaitsItsTurn(t *testing.T) {
	queue := newLineQueue(t)
	f := newFixture(t, queue)
	release := make(chan struct{})
	queue.Add(func() { <-release })
	done := make(chan error, 1)
	go func() {
		done <- f.store.CommitAndWait(context.Background(), "job", Insert("Things", Row{"Name": "cap"}))
	}()
	select {
	case err := <-done:
		t.Fatalf("returned ahead of earlier queued work: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if f.store.Count("Things", Row{"Name": "cap"}) != 1 {
		t.Fatal("the memory half waited on the queue")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if things := f.rows(t, "Things"); len(things) != 3 || things[2]["Name"] != "cap" {
		t.Fatalf("returned before the row was written: %v", things)
	}
	if err := f.store.CommitAndWait(context.Background(), "job", Insert("Things", Row{"Name": "sock", "Weight": "1"})); err == nil {
		t.Fatal("a failed write was not reported")
	}
}

func TestCascadeCarriesARename(t *testing.T) {
	f := newFixture(t, syncQueue{})
	if err := f.store.Commit(context.Background(), "ann", Update("Things", Row{"Name": "hat"}, Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	if f.store.Count("Uses", Row{"Thing": "cap"}) != 2 || f.store.Count("Uses", Row{"Thing": "hat"}) != 0 {
		t.Fatal("the uses did not follow the rename in memory")
	}
	for _, row := range f.rows(t, "Uses") {
		if row["Thing"] == "hat" {
			t.Fatalf("the sheet kept %v", row)
		}
	}
	equal(t, "change log", f.log(t), []string{
		"ann||set|Things|Name=cap|Name|hat",
		"ann||set|Uses|Thing=cap; By=ann|Thing|hat",
		"ann||set|Uses|Thing=cap; By=bo|Thing|hat",
	})
}

func TestRefusedChangeWritesNothing(t *testing.T) {
	f := newFixture(t, syncQueue{})
	if err := f.store.Commit(context.Background(), "ann", Update("Things", Row{"Name": "hat"}, Row{"Color": "plaid"})); err == nil {
		t.Fatal("a change the model refuses was taken")
	}
	if f.store.Count("Things", Row{"Color": "plaid"}) != 0 || f.rows(t, "Things")[0]["Color"] != "red" || len(f.log(t)) != 0 {
		t.Fatal("a refused change reached memory, the sheet or the log")
	}
}

type countingWriter struct {
	*data.Dir
	calls []string
}

func (w *countingWriter) Insert(app, table string, rows []map[string]string) error {
	w.calls = append(w.calls, "insert "+table)
	return w.Dir.Insert(app, table, rows)
}

func (w *countingWriter) Set(app, table string, match, cells map[string]string) error {
	w.calls = append(w.calls, "set "+table)
	return w.Dir.Set(app, table, match, cells)
}

func (w *countingWriter) SetMany(app, table, keyColumn string, cells map[string]map[string]string) error {
	w.calls = append(w.calls, "setmany "+table)
	return w.Dir.SetMany(app, table, keyColumn, cells)
}

func (w *countingWriter) Delete(app, table string, match map[string]string) error {
	w.calls = append(w.calls, "delete "+table)
	return w.Dir.Delete(app, table, match)
}

func TestWritesToOneTabAreBatched(t *testing.T) {
	f := newFixture(t, syncQueue{})
	writer := &countingWriter{Dir: f.dir}
	s, err := New(spec(), f.dir, writer, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	err = s.Commit(context.Background(), "job",
		Insert("Things", Row{"Name": "cap"}),
		Insert("Things", Row{"Name": "sock"}),
		Update("Things", Row{"Name": "hat"}, Row{"Color": "green"}),
		Set("Things", Row{"Name": "boot"}, Row{"Size": "small"}),
		Set("Things", Row{"Name": "HAT"}, Row{"Size": "large"}),
		Update("Things", Row{"Name": "cap"}, Row{"Name": "beret"}),
		Delete("Uses", Row{"Thing": "boot"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "calls", writer.calls, []string{"insert Things", "setmany Things", "set Things", "delete Uses", "insert Change Log"})
	got := []string{}
	for _, row := range f.rows(t, "Things") {
		got = append(got, row["Name"]+"|"+row["Color"]+"|"+row["Size"])
	}
	equal(t, "sheet", got, []string{"hat|green|large", "boot|black|small", "beret||", "sock||"})
}

func TestRefreshKeepsANewerCommit(t *testing.T) {
	queue := &heldQueue{}
	f := newFixture(t, queue)
	f.store.Refresh()
	if err := f.store.Commit(context.Background(), "ann", Insert("Things", Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	if f.store.Count("Things", nil) != 3 {
		t.Fatal("the commit waited on the queue")
	}
	queue.run()
	if f.store.Count("Things", Row{"Name": "cap"}) != 1 {
		t.Fatal("a refresh read before the commit put the older sheet back")
	}
}
