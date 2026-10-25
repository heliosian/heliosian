package store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/data"
)

type counts struct {
	things, uses, report, reportBuilds int
	consents, reportAtConsent          int
}

func consent(_ context.Context, m *counts) error {
	m.consents++
	m.reportAtConsent = m.report
	return nil
}

func noCheck(context.Context, *counts, *counts) error {
	return nil
}

var testChanges = &ChangeLog{
	Tab:     "Change Log",
	Columns: []string{"Timestamp", "Actor", "Real Actor", "Action", "Tab", "Key", "Column", "Previous"},
	Row: func(c Change) Row {
		key := []string{}
		for _, column := range c.Key {
			key = append(key, column+"="+c.Named[column])
		}
		return Row{
			"Timestamp": c.At.Format(time.RFC3339), "Actor": c.Actor, "Real Actor": c.Real, "Action": c.Action,
			"Tab": c.Tab, "Key": strings.Join(key, "; "), "Column": c.Column, "Previous": c.Previous,
		}
	},
}

func commit(s *Store[counts], ctx context.Context, actor access.Actor, app string, ops ...Op) error {
	_, err := s.queue.Transact(ctx, actor, func(tx *Tx) error { return s.Stage(tx, app, ops...) })
	return err
}

func commitAndWait(s *Store[counts], ctx context.Context, actor access.Actor, app string, ops ...Op) error {
	done, err := s.queue.Transact(ctx, actor, func(tx *Tx) error { return s.Stage(tx, app, ops...) })
	if err != nil || done == nil {
		return err
	}
	<-done
	return nil
}

func count(s *Store[counts], app, tab string, match Row) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, row := range s.tables[app][tab] {
		if data.Matches(row, match) {
			n++
		}
	}
	return n
}

type fixture struct {
	dir   *data.Dir
	queue *Queue
	store *Store[counts]
}

func write(t *testing.T, root, app, tab, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, app), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, app, tab+".csv"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func part() Part[counts] {
	return Part[counts]{
		App: "app",
		Tabs: []Tab{
			{Name: "Things", Columns: []string{"Name", "Color", "Size"}, Key: []string{"Name"}},
			{Name: "Uses", Columns: []string{"Thing", "By"}, Key: []string{"Thing", "By"}},
			{Name: "Events", Columns: []string{"When", "What"}, Key: []string{"When"}, AppendOnly: true},
		},
		Changes: testChanges,
		Build: func(_ context.Context, tables Tables, m *counts) error {
			for _, row := range tables["Things"] {
				if row["Color"] == "plaid" {
					return errors.New("plaid is not a color")
				}
			}
			m.things, m.uses = len(tables["Things"]), len(tables["Uses"])
			return nil
		},
		Loaded: func(*counts, time.Duration) {},
	}
}

func reportPart() Part[counts] {
	return Part[counts]{
		App:     "report",
		Tabs:    []Tab{{Name: "Notes", Columns: []string{"Note"}, Key: []string{"Note"}}},
		Changes: testChanges,
		Build: func(_ context.Context, tables Tables, m *counts) error {
			m.report = len(tables["Notes"])
			m.reportBuilds++
			return nil
		},
		Loaded: func(*counts, time.Duration) {},
	}
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	write(t, root, "app", "Things", "Name,Color,Size\nhat,red,small\nboot,black,large\n")
	write(t, root, "app", "Uses", "Thing,By\nhat,ann\nhat,bo\nboot,ann\n")
	write(t, root, "app", "Events", "When,What\n1,made\n")
	write(t, root, "app", testChanges.Tab, strings.Join(testChanges.Columns, ",")+"\n")
	write(t, root, "report", "Notes", "Note\nfirst\n")
	write(t, root, "report", testChanges.Tab, strings.Join(testChanges.Columns, ",")+"\n")
	dir := &data.Dir{Root: root}
	queue := NewQueue()
	s, err := New([]Part[counts]{part()}, consent, noCheck, dir, dir, queue)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{dir: dir, queue: queue, store: s}
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
	for _, row := range f.rows(t, testChanges.Tab) {
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
	f := newFixture(t)
	ctx := signedIn("admin@example.org")
	err := commitAndWait(f.store, ctx, access.Actor{Email: "ann@example.org"}, "app",
		Insert("Things", Row{"Name": "cap", "Color": "blue"}),
		Update("Things", Row{"Name": "boot"}, Row{"Color": "brown", "Size": ""}),
		Delete("Uses", Row{"Thing": "boot"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if m := f.store.Model(); m.things != 3 || m.uses != 2 {
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

func TestUpdateMatchingNothingWritesNothing(t *testing.T) {
	f := newFixture(t)
	if err := commitAndWait(f.store, context.Background(), access.System("job"), "app", Update("Things", Row{"Name": "sock"}, Row{"Color": "grey"})); err != nil {
		t.Fatal(err)
	}
	if len(f.rows(t, "Things")) != 2 || len(f.log(t)) != 0 {
		t.Fatal("an update matching nothing wrote something")
	}
}

func TestAppendOnlyIsWrittenNotLogged(t *testing.T) {
	f := newFixture(t)
	if err := commitAndWait(f.store, context.Background(), access.System("job"), "app", Insert("Events", Row{"When": "2", "What": "sent"})); err != nil {
		t.Fatal(err)
	}
	if err := commitAndWait(f.store, context.Background(), access.System("job"), "app", Delete("Events", Row{"When": "1"})); err != nil {
		t.Fatal(err)
	}
	if events := f.rows(t, "Events"); len(events) != 1 || events[0]["What"] != "sent" {
		t.Fatalf("sheet %v", events)
	}
	if len(f.log(t)) != 0 {
		t.Fatalf("an append-only tab was logged: %v", f.log(t))
	}
	if err := commitAndWait(f.store, context.Background(), access.System("job"), "app", Update("Events", Row{"When": "2"}, Row{"What": "bounced"})); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("an edit of an append-only tab was taken: %v", err)
	}
}

func TestCommitAndWaitWaitsItsTurn(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	f.queue.Add(func() { <-release })
	done := make(chan error, 1)
	go func() {
		done <- commitAndWait(f.store, context.Background(), access.System("job"), "app", Insert("Things", Row{"Name": "cap"}))
	}()
	select {
	case err := <-done:
		t.Fatalf("returned ahead of earlier queued work: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if count(f.store, "app", "Things", Row{"Name": "cap"}) != 1 {
		t.Fatal("the memory half waited on the queue")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if things := f.rows(t, "Things"); len(things) != 3 || things[2]["Name"] != "cap" {
		t.Fatalf("returned before the row was written: %v", things)
	}
}

func TestRefusedWriteIsFatal(t *testing.T) {
	if os.Getenv("STORE_REFUSED_WRITE") == "1" {
		f := newFixture(t)
		commitAndWait(f.store, context.Background(), access.System("job"), "app", Insert("Things", Row{"Name": "sock", "Weight": "1"}))
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRefusedWriteIsFatal$")
	cmd.Env = append(os.Environ(), "STORE_REFUSED_WRITE=1")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(out), "missing column") {
		t.Fatalf("a refused write did not end the process: %v\n%s", err, out)
	}
}

func TestPaddedMatchReachesTheSheet(t *testing.T) {
	f := newFixture(t)
	if err := commitAndWait(f.store, context.Background(), access.System("job"), "app", Delete("Uses", Row{"Thing": " HAT ", "By": "bo "})); err != nil {
		t.Fatal(err)
	}
	if count(f.store, "app", "Uses", Row{"Thing": "hat"}) != 1 || len(f.rows(t, "Uses")) != 2 {
		t.Fatalf("memory and sheet split: memory %d, sheet %v", count(f.store, "app", "Uses", Row{"Thing": "hat"}), f.rows(t, "Uses"))
	}
}

func TestUnmatchedSheetWriteIsFatal(t *testing.T) {
	if op := os.Getenv("STORE_UNMATCHED_WRITE"); op != "" {
		f := newFixture(t)
		if err := f.dir.Delete("app", "Uses", Row{"Thing": "boot"}); err != nil {
			t.Fatal(err)
		}
		switch op {
		case "update":
			commitAndWait(f.store, context.Background(), access.System("job"), "app", Update("Uses", Row{"Thing": "boot", "By": "ann"}, Row{"By": "bo"}))
		case "delete":
			commitAndWait(f.store, context.Background(), access.System("job"), "app", Delete("Uses", Row{"Thing": "boot"}))
		}
		return
	}
	for _, op := range []string{"update", "delete"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestUnmatchedSheetWriteIsFatal$")
		cmd.Env = append(os.Environ(), "STORE_UNMATCHED_WRITE="+op)
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(out), "no row matching") {
			t.Fatalf("an unmatched %s did not end the process: %v\n%s", op, err, out)
		}
	}
}

func TestRefusedChangeWritesNothing(t *testing.T) {
	f := newFixture(t)
	if err := commitAndWait(f.store, context.Background(), access.Actor{Email: "ann"}, "app", Update("Things", Row{"Name": "hat"}, Row{"Color": "plaid"})); err == nil {
		t.Fatal("a change the model refuses was taken")
	}
	if count(f.store, "app", "Things", Row{"Color": "plaid"}) != 0 || f.rows(t, "Things")[0]["Color"] != "red" || len(f.log(t)) != 0 {
		t.Fatal("a refused change reached memory, the sheet or the log")
	}
}

func TestATransactionCommitsEveryStageOrNone(t *testing.T) {
	f := newFixture(t)
	ran, swapped := false, 0
	_, err := f.queue.Transact(context.Background(), access.Actor{Email: "ann"}, func(tx *Tx) error {
		tx.After(func() { ran = true })
		tx.Swapped(func() { swapped = f.store.Model().things })
		if err := f.store.Stage(tx, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
			return err
		}
		if f.store.In(tx).things != 3 || f.store.Model().things != 2 {
			t.Errorf("staged %v, current %v", f.store.In(tx), f.store.Model())
		}
		return f.store.Stage(tx, "app", Update("Things", Row{"Name": "hat"}, Row{"Color": "plaid"}))
	})
	if err == nil {
		t.Fatal("a transaction with a refused stage was taken")
	}
	if ran || swapped != 0 || count(f.store, "app", "Things", Row{"Name": "cap"}) != 0 || len(f.rows(t, "Things")) != 2 || len(f.log(t)) != 0 {
		t.Fatal("a refused transaction reached memory, the sheet, the log or its after-work")
	}
	done, err := f.queue.Transact(context.Background(), access.Actor{Email: "ann"}, func(tx *Tx) error {
		tx.After(func() { ran = true })
		tx.Swapped(func() { swapped = f.store.Model().things })
		if err := f.store.Stage(tx, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
			return err
		}
		return f.store.Stage(tx, "app", Update("Things", Row{"Name": "cap"}, Row{"Color": "blue"}))
	})
	if err != nil {
		t.Fatal(err)
	}
	<-done
	if !ran || swapped != 3 || count(f.store, "app", "Things", Row{"Name": "cap", "Color": "blue"}) != 1 {
		t.Fatalf("a transaction's stages did not all land, or its swap work saw %d things, not 3", swapped)
	}
	equal(t, "change log", f.log(t), []string{"ann||insert|Things|Name=cap||", "ann||set|Things|Name=cap|Color|"})
}

func TestATransactionIsCheckedWhole(t *testing.T) {
	f := newFixture(t)
	atMostTwo := func(_ context.Context, _, m *counts) error {
		if m.things > 2 {
			return errors.New("three things")
		}
		return nil
	}
	s, err := New([]Part[counts]{part()}, consent, atMostTwo, f.dir, f.dir, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	done, err := f.queue.Transact(context.Background(), access.Actor{Email: "ann"}, func(tx *Tx) error {
		if err := s.Stage(tx, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
			return err
		}
		return s.Stage(tx, "app", Delete("Things", Row{"Name": "boot"}))
	})
	if err != nil {
		t.Fatalf("a transaction that ends within the check was refused for a state between its stages: %v", err)
	}
	<-done
	if s.Model().things != 2 || count(s, "app", "Things", Row{"Name": "cap"}) != 1 {
		t.Fatalf("after the transaction: %+v", *s.Model())
	}
	if err := commit(s, context.Background(), access.Actor{Email: "ann"}, "app", Insert("Things", Row{"Name": "sock"})); err == nil || !strings.Contains(err.Error(), "three things") {
		t.Fatalf("a transaction ending outside the check: %v", err)
	}
}

func reportFixture(t *testing.T) (fixture, *Store[counts]) {
	t.Helper()
	f := newFixture(t)
	s, err := New([]Part[counts]{reportPart(), part()}, consent, noCheck, f.dir, f.dir, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	return f, s
}

func TestACommitRebuildsOnlyThePartItChanged(t *testing.T) {
	f, s := reportFixture(t)
	if m := s.Model(); m.report != 1 || m.reportBuilds != 1 {
		t.Fatalf("loaded %+v", *m)
	}
	if err := commitAndWait(s, context.Background(), access.System("job"), "app", Insert("Things", Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	if m := s.Model(); m.things != 3 || m.report != 1 || m.reportBuilds != 1 {
		t.Fatalf("after a change to another part: %+v", *m)
	}
	if err := commitAndWait(s, context.Background(), access.System("job"), "report", Insert("Notes", Row{"Note": "second"})); err != nil {
		t.Fatal(err)
	}
	if m := s.Model(); m.report != 2 || m.reportBuilds != 2 {
		t.Fatalf("after a change to the report's own sheet: %+v", *m)
	}
	if _, rows, _ := f.dir.Table("report", "Notes"); len(rows) != 2 {
		t.Fatalf("the report's sheet holds %v", rows)
	}
}

func TestConsentRunsAfterEveryPart(t *testing.T) {
	_, s := reportFixture(t)
	if m := s.Model(); m.consents != 1 || m.reportAtConsent != 1 {
		t.Fatalf("loaded %+v", *m)
	}
	if err := commitAndWait(s, context.Background(), access.System("job"), "report", Insert("Notes", Row{"Note": "second"})); err != nil {
		t.Fatal(err)
	}
	if m := s.Model(); m.consents != 2 || m.reportAtConsent != 2 {
		t.Fatalf("after a change: %+v", *m)
	}
}

func TestConsentThatRefusesRefusesTheWrite(t *testing.T) {
	f := newFixture(t)
	refusing := func(_ context.Context, m *counts) error {
		if m.things > 2 {
			return errors.New("three things")
		}
		return nil
	}
	s, err := New([]Part[counts]{part()}, refusing, noCheck, f.dir, f.dir, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	err = commitAndWait(s, context.Background(), access.System("job"), "app", Insert("Things", Row{"Name": "cap"}))
	if err == nil || !strings.Contains(err.Error(), "three things") {
		t.Fatalf("a write the consent step refuses: %v", err)
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

func (w *countingWriter) Update(app, table string, match, cells map[string]string) error {
	w.calls = append(w.calls, "update "+table)
	return w.Dir.Update(app, table, match, cells)
}

func (w *countingWriter) SetMany(app, table, keyColumn string, cells map[string]map[string]string) error {
	w.calls = append(w.calls, "setmany "+table)
	return w.Dir.SetMany(app, table, keyColumn, cells)
}

func (w *countingWriter) Delete(app, table string, match map[string]string) error {
	w.calls = append(w.calls, "delete "+table)
	return w.Dir.Delete(app, table, match)
}

func (w *countingWriter) DeleteMany(app, table, keyColumn string, keys []string) error {
	w.calls = append(w.calls, "delete "+table)
	return w.Dir.DeleteMany(app, table, keyColumn, keys)
}

func TestConsecutiveDeletesAreOneCall(t *testing.T) {
	f := newFixture(t)
	writer := &countingWriter{Dir: f.dir}
	s, err := New([]Part[counts]{part()}, consent, noCheck, f.dir, writer, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	err = commitAndWait(s, context.Background(), access.System("job"), "app",
		Delete("Uses", Row{"Thing": "boot"}),
		Delete("Uses", Row{"Thing": "hat"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "calls", writer.calls, []string{"delete Uses", "insert Change Log"})
	if rows := f.rows(t, "Uses"); len(rows) != 0 {
		t.Fatalf("uses left: %v", rows)
	}
}

func TestWritesToOneTabAreBatched(t *testing.T) {
	f := newFixture(t)
	writer := &countingWriter{Dir: f.dir}
	s, err := New([]Part[counts]{part()}, consent, noCheck, f.dir, writer, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	err = commitAndWait(s, context.Background(), access.System("job"), "app",
		Insert("Things", Row{"Name": "cap"}),
		Insert("Things", Row{"Name": "sock"}),
		Update("Things", Row{"Name": "hat"}, Row{"Color": "green"}),
		Update("Things", Row{"Name": "boot"}, Row{"Size": "small"}),
		Update("Things", Row{"Name": "HAT"}, Row{"Size": "large"}),
		Update("Things", Row{"Name": "cap"}, Row{"Name": "beret"}),
		Delete("Uses", Row{"Thing": "boot"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	equal(t, "calls", writer.calls, []string{"insert Things", "setmany Things", "update Things", "delete Uses", "insert Change Log"})
	got := []string{}
	for _, row := range f.rows(t, "Things") {
		got = append(got, row["Name"]+"|"+row["Color"]+"|"+row["Size"])
	}
	equal(t, "sheet", got, []string{"hat|green|large", "boot|black|small", "beret||", "sock||"})
}

func TestWaitingCommitsAreWrittenTogether(t *testing.T) {
	f := newFixture(t)
	writer := &countingWriter{Dir: f.dir}
	s, err := New([]Part[counts]{part()}, consent, noCheck, f.dir, writer, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	f.queue.Add(func() { <-release })
	dones := []<-chan struct{}{}
	for _, ops := range [][]Op{
		{Insert("Things", Row{"Name": "cap"}), Insert("Uses", Row{"Thing": "cap", "By": "cy"})},
		{Insert("Things", Row{"Name": "sock"}), Insert("Uses", Row{"Thing": "sock", "By": "cy"})},
		{Update("Things", Row{"Name": "hat"}, Row{"Color": "green"})},
	} {
		done, err := s.queue.Transact(context.Background(), access.System("job"), func(tx *Tx) error { return s.Stage(tx, "app", ops...) })
		if err != nil {
			t.Fatal(err)
		}
		dones = append(dones, done)
	}
	close(release)
	for _, done := range dones {
		<-done
	}
	equal(t, "calls", writer.calls, []string{"insert Things", "insert Uses", "setmany Things", "insert Change Log"})
	things, uses := []string{}, []string{}
	for _, row := range f.rows(t, "Things") {
		things = append(things, row["Name"]+"|"+row["Color"])
	}
	for _, row := range f.rows(t, "Uses") {
		uses = append(uses, row["Thing"]+"|"+row["By"])
	}
	equal(t, "things", things, []string{"hat|green", "boot|black", "cap|", "sock|"})
	equal(t, "uses", uses, []string{"hat|ann", "hat|bo", "boot|ann", "cap|cy", "sock|cy"})
	if n := len(f.log(t)); n != 5 {
		t.Fatalf("the change log holds %d entries", n)
	}
}

func TestBatchingReachesPastOtherTabs(t *testing.T) {
	for _, c := range []struct {
		name   string
		ops    []Op
		calls  []string
		things []string
		uses   []string
	}{
		{
			name: "interleaved tabs collapse to one call each",
			ops: []Op{
				Insert("Things", Row{"Name": "cap"}), Insert("Uses", Row{"Thing": "hat", "By": "cy"}),
				Insert("Things", Row{"Name": "sock"}), Insert("Uses", Row{"Thing": "boot", "By": "cy"}),
				Update("Things", Row{"Name": "hat"}, Row{"Color": "green"}), Insert("Things", Row{"Name": "scarf"}),
				Update("Things", Row{"Name": "boot"}, Row{"Color": "brown"}),
			},
			calls:  []string{"insert Things", "insert Uses", "setmany Things", "insert Change Log"},
			things: []string{"hat|green", "boot|brown", "cap|", "sock|", "scarf|"},
			uses:   []string{"hat|ann", "hat|bo", "boot|ann", "hat|cy", "boot|cy"},
		},
		{
			name:   "a delete holds back a later insert of the row it matched",
			ops:    []Op{Insert("Things", Row{"Name": "cap"}), Delete("Things", Row{"Name": "cap"}), Insert("Things", Row{"Name": "cap", "Color": "blue"})},
			calls:  []string{"insert Things", "delete Things", "insert Things", "insert Change Log"},
			things: []string{"hat|red", "boot|black", "cap|blue"},
			uses:   []string{"hat|ann", "hat|bo", "boot|ann"},
		},
		{
			name:   "a row naming another tab's new row waits for it",
			ops:    []Op{Insert("Uses", Row{"Thing": "hat", "By": "cy"}), Insert("Things", Row{"Name": "newt"}), Insert("Uses", Row{"Thing": "newt", "By": "cy"})},
			calls:  []string{"insert Uses", "insert Things", "insert Uses", "insert Change Log"},
			things: []string{"hat|red", "boot|black", "newt|"},
			uses:   []string{"hat|ann", "hat|bo", "boot|ann", "hat|cy", "newt|cy"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			writer := &countingWriter{Dir: f.dir}
			s, err := New([]Part[counts]{part()}, consent, noCheck, f.dir, writer, f.queue)
			if err != nil {
				t.Fatal(err)
			}
			if err := commitAndWait(s, context.Background(), access.System("job"), "app", c.ops...); err != nil {
				t.Fatal(err)
			}
			equal(t, "calls", writer.calls, c.calls)
			things, uses := []string{}, []string{}
			for _, row := range f.rows(t, "Things") {
				things = append(things, row["Name"]+"|"+row["Color"])
			}
			for _, row := range f.rows(t, "Uses") {
				uses = append(uses, row["Thing"]+"|"+row["By"])
			}
			equal(t, "things", things, c.things)
			equal(t, "uses", uses, c.uses)
		})
	}
}

type pausedSource struct {
	*data.Dir
	pause   *atomic.Bool
	reading chan struct{}
	release chan struct{}
}

func (p pausedSource) Tabs(ctx context.Context, app string, tables, headers []string) (map[string]data.Tab, error) {
	if p.pause.Load() {
		p.reading <- struct{}{}
		<-p.release
	}
	return p.Dir.Tabs(ctx, app, tables, headers)
}

func TestACommitDuringARefreshIsKeptAndTheRefreshSwaps(t *testing.T) {
	f := newFixture(t)
	paused := pausedSource{Dir: f.dir, pause: &atomic.Bool{}, reading: make(chan struct{}), release: make(chan struct{})}
	other, err := New([]Part[counts]{part()}, consent, noCheck, paused, f.dir, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	paused.pause.Store(true)
	if err := f.dir.Delete("app", "Uses", Row{"By": "bo"}); err != nil {
		t.Fatal(err)
	}
	f.queue.Refresh()
	<-paused.reading
	if err := commit(f.store, context.Background(), access.Actor{Email: "ann"}, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	close(paused.release)
	f.queue.Flush()
	if count(f.store, "app", "Things", Row{"Name": "cap"}) != 1 || f.store.Model().things != 3 {
		t.Fatal("a refresh read before the commit put the older sheet back")
	}
	if f.store.Model().uses != 2 || other.Model().uses != 2 {
		t.Fatal("a refresh a commit landed in did not swap its read in")
	}
	if len(f.rows(t, "Things")) != 3 {
		t.Fatalf("the sheet after the refresh: %v", f.rows(t, "Things"))
	}
}

func TestARefreshReportsTheRowsTheSheetChanged(t *testing.T) {
	f := newFixture(t)
	seen := []string{}
	p := part()
	p.Refreshed = func(tab string, before, after Row) {
		seen = append(seen, tab+" "+before["Name"]+before["Thing"]+before["By"]+" -> "+after["Name"]+after["Thing"]+after["By"])
	}
	s, err := New([]Part[counts]{p}, consent, noCheck, f.dir, f.dir, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitAndWait(s, context.Background(), access.Actor{Email: "ann"}, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	if err := f.dir.Insert("app", "Things", []map[string]string{{"Name": "sock"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.dir.Delete("app", "Uses", Row{"By": "bo"}); err != nil {
		t.Fatal(err)
	}
	f.queue.Refresh()
	f.queue.Flush()
	equal(t, "refreshed rows", seen, []string{"Things  -> sock", "Uses hatbo -> "})
}

func TestARefreshRebuildsOnlyThePartsWhoseSheetsChanged(t *testing.T) {
	f := newFixture(t)
	app, report := part(), reportPart()
	appBuilds := 0
	build := app.Build
	app.Build = func(ctx context.Context, tables Tables, m *counts) error {
		appBuilds++
		return build(ctx, tables, m)
	}
	s, err := New([]Part[counts]{app, report}, consent, noCheck, f.dir, f.dir, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	refresh := func() *counts {
		t.Helper()
		f.queue.Refresh()
		f.queue.Flush()
		return s.Model()
	}
	loaded := s.Model()
	if m := refresh(); m != loaded || appBuilds != 1 || m.reportBuilds != 1 {
		t.Fatalf("a refresh of unchanged sheets rebuilt: same model %v, app built %d times, report %d", m == loaded, appBuilds, m.reportBuilds)
	}
	if err := f.dir.Insert("report", "Notes", []map[string]string{{"Note": "second"}}); err != nil {
		t.Fatal(err)
	}
	if m := refresh(); appBuilds != 1 || m.reportBuilds != 2 || m.report != 2 {
		t.Fatalf("a new note rebuilt app %d times and report %d times, report %d", appBuilds, m.reportBuilds, m.report)
	}
	if err := f.dir.Insert("app", "Things", []map[string]string{{"Name": "sock"}}); err != nil {
		t.Fatal(err)
	}
	if m := refresh(); appBuilds != 2 || m.reportBuilds != 2 || m.things != 3 {
		t.Fatalf("a new thing rebuilt app %d times and report %d times, things %d", appBuilds, m.reportBuilds, m.things)
	}
}

func TestARefreshThatCannotTakeACommitKeepsTheModel(t *testing.T) {
	f := newFixture(t)
	paused := pausedSource{Dir: f.dir, pause: &atomic.Bool{}, reading: make(chan struct{}), release: make(chan struct{})}
	atMostThree := func(_ context.Context, _, m *counts) error {
		if m.things > 3 {
			return errors.New("four things")
		}
		return nil
	}
	s, err := New([]Part[counts]{part()}, consent, atMostThree, paused, f.dir, f.queue)
	if err != nil {
		t.Fatal(err)
	}
	paused.pause.Store(true)
	f.queue.Refresh()
	<-paused.reading
	if err := f.dir.Insert("app", "Things", []map[string]string{{"Name": "sock"}}); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, context.Background(), access.Actor{Email: "ann"}, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	paused.release <- struct{}{}
	f.queue.Flush()
	if m := s.Model(); m.things != 3 {
		t.Fatalf("a refresh the commit could not be replayed on swapped in: %+v", *m)
	}
	select {
	case <-f.queue.Refreshed():
		t.Fatal("a refresh that did not swap counted as the first")
	default:
	}
}

func TestACommitHeldOverARefreshIsKept(t *testing.T) {
	f := newFixture(t)
	if err := f.dir.Delete("app", "Uses", Row{"By": "bo"}); err != nil {
		t.Fatal(err)
	}
	entered, hold := make(chan struct{}), make(chan struct{})
	committed := make(chan error, 1)
	go func() {
		_, err := f.queue.Transact(context.Background(), access.Actor{Email: "ann"}, func(tx *Tx) error {
			close(entered)
			<-hold
			return f.store.Stage(tx, "app", Insert("Things", Row{"Name": "cap"}))
		})
		committed <- err
	}()
	<-entered
	f.queue.Refresh()
	time.Sleep(50 * time.Millisecond)
	close(hold)
	if err := <-committed; err != nil {
		t.Fatal(err)
	}
	f.queue.Flush()
	if count(f.store, "app", "Things", Row{"Name": "cap"}) != 1 || f.store.Model().uses != 2 {
		t.Fatal("a refresh waiting out a commit put the older sheet back")
	}
}

func TestAWriteWaitingBehindARefreshIsKept(t *testing.T) {
	f := newFixture(t)
	if err := f.dir.Delete("app", "Uses", Row{"By": "bo"}); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	f.queue.Add(func() { <-release })
	f.queue.Refresh()
	if err := commit(f.store, context.Background(), access.Actor{Email: "ann"}, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	close(release)
	f.queue.Flush()
	if count(f.store, "app", "Things", Row{"Name": "cap"}) != 1 || f.store.Model().uses != 2 {
		t.Fatal("a refresh ahead of a commit's write put the older sheet back")
	}
	if len(f.rows(t, "Things")) != 3 {
		t.Fatalf("the sheet after the refresh: %v", f.rows(t, "Things"))
	}
}

func TestRefreshedClosesWithTheFirstRefreshDespiteWrites(t *testing.T) {
	f := newFixture(t)
	release := make(chan struct{})
	f.queue.Add(func() { <-release })
	f.queue.Refresh()
	if err := commit(f.store, context.Background(), access.Actor{Email: "ann"}, "app", Insert("Things", Row{"Name": "cap"})); err != nil {
		t.Fatal(err)
	}
	close(release)
	f.queue.Flush()
	select {
	case <-f.queue.Refreshed():
	default:
		t.Fatal("a refresh with a write waiting behind it did not count")
	}
}

func TestARefreshSwapsEveryPartOrNone(t *testing.T) {
	f, s := reportFixture(t)
	if err := f.dir.Delete("app", "Uses", Row{"By": "bo"}); err != nil {
		t.Fatal(err)
	}
	if err := f.dir.Insert("report", "Notes", []map[string]string{{"Note": "second"}}); err != nil {
		t.Fatal(err)
	}
	f.queue.Refresh()
	f.queue.Flush()
	if m := s.Model(); m.uses != 2 || m.report != 2 {
		t.Fatalf("a refresh missed a part: %+v", *m)
	}
	if err := f.dir.Insert("report", "Notes", []map[string]string{{"Note": "third"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.dir.Delete("app", "Uses", Row{"By": "ann"}); err != nil {
		t.Fatal(err)
	}
	if err := f.dir.Update("app", "Things", Row{"Name": "hat"}, Row{"Color": "plaid"}); err != nil {
		t.Fatal(err)
	}
	f.queue.Refresh()
	f.queue.Flush()
	if m := s.Model(); m.uses != 2 || m.report != 2 {
		t.Fatalf("a refresh one part refused swapped the others in: %+v", *m)
	}
}
