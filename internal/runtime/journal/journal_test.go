package journal

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCodecRoundTripKeepsTypes(t *testing.T) {
	sp := time.FixedZone("-03", -3*3600)
	in := map[string]any{
		"int":      int64(2),
		"float":    2.0,
		"small":    1e-9,
		"big":      uint64(math.MaxUint64),
		"time":     time.Date(2026, 9, 23, 14, 30, 0, 123, sp),
		"duration": 90 * time.Minute,
		"bytes":    []byte{0, 1, 2},
		"list":     []any{int64(1), "a", nil, true, []any{}},
		"$weird":   "kept",
		"$$more":   "kept too",
		"nested":   map[string]any{"$time": int64(5)}, // not a tag: not a string
	}
	got, err := Normalize(in)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if _, ok := m["int"].(int64); !ok {
		t.Errorf("int became %T", m["int"])
	}
	if _, ok := m["float"].(float64); !ok {
		t.Errorf("integral float became %T", m["float"])
	}
	if tm := m["time"].(time.Time); !tm.Equal(in["time"].(time.Time)) {
		t.Errorf("time %v", tm)
	} else if _, off := tm.Zone(); off != -3*3600 {
		t.Errorf("time lost its offset: %v", tm)
	}
	delete(m, "time")
	want := map[string]any{}
	for k, v := range in {
		want[k] = v
	}
	delete(want, "time")
	if !reflect.DeepEqual(m, want) {
		t.Errorf("round trip:\n got %#v\nwant %#v", m, want)
	}
}

func TestCodecIsCanonical(t *testing.T) {
	a, _ := Encode(map[string]any{"b": int64(1), "a": []any{2.5}})
	b, _ := Encode(map[string]any{"a": []any{2.5}, "b": int64(1)})
	if string(a) != string(b) || string(a) != `{"a":[2.5],"b":1}` {
		t.Errorf("%s vs %s", a, b)
	}
}

func TestCodecRefuses(t *testing.T) {
	for _, v := range []any{math.NaN(), math.Inf(1), struct{}{}, map[int]any{}} {
		if _, err := Encode(v); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%T %v: want ErrUnsupported, got %v", v, v, err)
		}
	}
	deep := any("x")
	for range maxDepth + 2 {
		deep = []any{deep}
	}
	if _, err := Encode(deep); !errors.Is(err, ErrUnsupported) {
		t.Errorf("deep value: %v", err)
	}
	for _, bad := range []string{`{"$nope":1}`, `1 2`, `{"$time":"yesterday"}`} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Errorf("%s must not decode", bad)
		}
	}
}

// storeContract runs the Store rules every implementation must follow.
func storeContract(t *testing.T, m Store) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 5, time.UTC)
	if err := m.CreateRun(ctx, Run{ID: "r1", Scopes: []byte("{}"), Status: StatusResolving, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateRun(ctx, Run{ID: "r1", Scopes: []byte("{}")}); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate run: %v", err)
	}
	if _, err := m.GetRun(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown run: %v", err)
	}
	if seq, err := m.Append(ctx, "r1", Record{Site: "a", Kind: KindResult, Value: []byte(`1`), At: now}); err != nil || seq != 1 {
		t.Fatalf("append: %d %v", seq, err)
	}
	if _, err := m.Append(ctx, "r1", Record{Site: "a", Kind: KindResult, At: now}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("same site and kind twice: %v", err)
	}
	if seq, err := m.Append(ctx, "r1", Record{Site: "a", Kind: KindIntent, At: now}); err != nil || seq != 2 {
		t.Errorf("same site, other kind: %d %v", seq, err)
	}
	if _, err := m.Append(ctx, "nope", Record{Site: "a", At: now}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown run: %v", err)
	}
	rs, err := m.Records(ctx, "r1")
	if err != nil || len(rs) != 2 || rs[0].Site != "a" || string(rs[0].Value) != "1" || !rs[0].At.Equal(now) || rs[1].Seq != 2 {
		t.Errorf("records %+v %v", rs, err)
	}
	if err := m.UpdateRun(ctx, "r1", StatusWaitingForUser, "x"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateRun(ctx, "nope", StatusFailed, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("update unknown: %v", err)
	}
	for _, seq := range []int{2, 1} { // Ack never moves backwards
		if err := m.Ack(ctx, "r1", seq); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.CreateRun(ctx, Run{ID: "r2", Conversation: "c", Scopes: []byte("{}"), Status: StatusSucceeded, CreatedAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateRun(ctx, Run{ID: "r3", Conversation: "c", Scopes: []byte("{}"), Status: StatusWaitingForUser, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if a, ok, err := m.ActiveRun(ctx, "c"); err != nil || !ok || a.ID != "r3" {
		t.Errorf("active run = %v %v %v; finished runs never count", a.ID, ok, err)
	}
	if _, ok, err := m.ActiveRun(ctx, "other"); err != nil || ok {
		t.Errorf("no active run expected: %v %v", ok, err)
	}
	if l, ok, err := m.LastRun(ctx, "c"); err != nil || !ok || l.ID != "r2" {
		t.Errorf("last run = %v %v %v; finished runs count here", l.ID, ok, err)
	}
	r, err := m.GetRun(ctx, "r1")
	if err != nil || r.Status != StatusWaitingForUser || r.Pending != "x" || r.Acked != 2 || string(r.Scopes) != "{}" || !r.CreatedAt.Equal(now) {
		t.Errorf("run = %+v %v", r, err)
	}
}

func TestMemoryStore(t *testing.T) {
	m := NewMemory()
	storeContract(t, m)
	m.FailAppendAfter = 1
	if _, err := m.Append(context.Background(), "r1", Record{Site: "b", Kind: KindResult}); !errors.Is(err, ErrInjected) {
		t.Errorf("injected: %v", err)
	}
	if rs, _ := m.Records(context.Background(), "r1"); len(rs) != 2 {
		t.Errorf("records = %d, the injected failure must not be stored", len(rs))
	}
}

func TestSQLiteStore(t *testing.T) {
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	storeContract(t, s)
}

func TestSQLiteSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	ctx := context.Background()
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRun(ctx, Run{ID: "r", Scopes: []byte("{}"), Status: StatusResolving}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(ctx, "r", Record{Site: "action", Kind: KindIntent}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	if rs, err := s.Records(ctx, "r"); err != nil || len(rs) != 1 || rs[0].Kind != KindIntent {
		t.Errorf("after reopen: %+v %v", rs, err)
	}
}

// Text order must be time order: with trimmed fractions, 12:00:05Z would
// sort after 12:00:05.1Z and the older run would look newer.
func TestSQLiteActiveRunOrdersByTime(t *testing.T) {
	s, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	base := time.Date(2026, 9, 23, 12, 0, 5, 0, time.UTC)
	for id, at := range map[string]time.Time{"older": base, "newer": base.Add(100 * time.Millisecond)} {
		if err := s.CreateRun(ctx, Run{ID: id, Conversation: "c", Scopes: []byte("{}"), Status: StatusWaitingForUser, CreatedAt: at, UpdatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	if r, ok, err := s.ActiveRun(ctx, "c"); err != nil || !ok || r.ID != "newer" {
		t.Errorf("active run %q %v %v", r.ID, ok, err)
	}
}

// With equal creation times (a fixed test clock, or one clock tick), the
// run created later is the last one, whatever its ID. Found when the
// router's context came from a random run in CI.
func TestLastRunTiesBreakByInsertion(t *testing.T) {
	sqlite, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlite.Close() }()
	for name, st := range map[string]Store{"memory": NewMemory(), "sqlite": sqlite} {
		ctx := context.Background()
		at := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
		for _, id := range []string{"zzz-first", "aaa-second"} {
			if err := st.CreateRun(ctx, Run{ID: id, Conversation: "c", Scopes: []byte("{}"), Status: StatusWaitingForUser, CreatedAt: at, UpdatedAt: at}); err != nil {
				t.Fatal(err)
			}
		}
		if r, _, _ := st.LastRun(ctx, "c"); r.ID != "aaa-second" {
			t.Errorf("%s: last run %q, want the one created last", name, r.ID)
		}
		if r, _, _ := st.ActiveRun(ctx, "c"); r.ID != "aaa-second" {
			t.Errorf("%s: active run %q, want the one created last", name, r.ID)
		}
	}
}

// Runs lists newest first, with filters and paging, on both stores.
func TestRunsFilter(t *testing.T) {
	sqlite, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlite.Close() }()
	for name, st := range map[string]Store{"memory": NewMemory(), "sqlite": sqlite} {
		ctx := context.Background()
		base := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
		for i, r := range []struct{ conv, fn string }{{"a", "f"}, {"b", "g"}, {"a", "g"}, {"a", "f"}} {
			at := base.Add(time.Duration(i) * time.Minute)
			status := StatusSucceeded
			if i == 3 {
				status = StatusFailed
			}
			if err := st.CreateRun(ctx, Run{ID: fmt.Sprintf("r%d", i), Namespace: "ns", Function: r.fn, Conversation: r.conv, Scopes: []byte("{}"), Status: status, CreatedAt: at, UpdatedAt: at}); err != nil {
				t.Fatal(err)
			}
		}
		ids := func(f RunFilter) string {
			rs, err := st.Runs(ctx, f)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			for _, r := range rs {
				out = append(out, r.ID)
			}
			return strings.Join(out, ",")
		}
		for _, c := range []struct {
			f    RunFilter
			want string
		}{
			{RunFilter{}, "r3,r2,r1,r0"},
			{RunFilter{Conversation: "a"}, "r3,r2,r0"},
			{RunFilter{Function: "g"}, "r2,r1"},
			{RunFilter{Status: StatusFailed}, "r3"},
			{RunFilter{Limit: 2}, "r3,r2"},
			{RunFilter{Before: base.Add(2 * time.Minute)}, "r1,r0"},
			{RunFilter{Namespace: "other"}, ""},
		} {
			if got := ids(c.f); got != c.want {
				t.Errorf("%s %+v: %s, want %s", name, c.f, got, c.want)
			}
		}
	}
}

// Unacked with a prefix keeps only that channel's conversations, in both
// stores, so other channels' unread runs never fill its page.
func TestUnackedPrefix(t *testing.T) {
	sq, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	for name, s := range map[string]Store{"memory": NewMemory(), "sqlite": sq} {
		ctx := context.Background()
		for _, c := range []string{"console-1", "wa:5511", "console-2"} {
			if err := s.CreateRun(ctx, Run{ID: c, Namespace: "ns", Function: "f", Conversation: c, Status: StatusWaitingForUser, Scopes: []byte("{}")}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Append(ctx, c, Record{Site: "m", Kind: KindSend, Value: []byte(`{}`)}); err != nil {
				t.Fatal(err)
			}
		}
		runs, err := s.Unacked(ctx, "wa:", 1)
		if err != nil || len(runs) != 1 || runs[0].ID != "wa:5511" {
			t.Errorf("%s: %+v %v", name, runs, err)
		}
		if all, _ := s.Unacked(ctx, "", 10); len(all) != 3 {
			t.Errorf("%s: no prefix, every run: %d", name, len(all))
		}
	}
}

func TestWatches(t *testing.T) {
	sq, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	for name, s := range map[string]Store{"memory": NewMemory(), "sqlite": sq} {
		ctx := context.Background()
		w := Watch{RunID: "r1", Namespace: "ns", Site: "w", Event: "booked", Key: "L1"}
		for range 2 { // idempotent
			if err := s.Watch(ctx, w); err != nil {
				t.Fatal(err)
			}
		}
		_ = s.Watch(ctx, Watch{RunID: "r2", Namespace: "ns", Site: "w", Event: "booked", Key: "L2"})
		got, err := s.Watchers(ctx, "ns", "booked", "L1")
		if err != nil || len(got) != 1 || got[0] != w {
			t.Errorf("%s: %+v %v", name, got, err)
		}
		if err := s.Unwatch(ctx, "r1"); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Watchers(ctx, "ns", "booked", "L1"); len(got) != 0 {
			t.Errorf("%s: after Unwatch %+v", name, got)
		}
		if got, _ := s.Watchers(ctx, "ns", "booked", "L2"); len(got) != 1 {
			t.Errorf("%s: another run's watch is kept: %+v", name, got)
		}
	}
}

func TestMilestonesAfterAndCursors(t *testing.T) {
	sq, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	for name, s := range map[string]Store{"memory": NewMemory(), "sqlite": sq} {
		ctx := context.Background()
		if last, _ := s.LastMilestone(ctx, "ns"); last != 0 {
			t.Errorf("%s: empty: %d", name, last)
		}
		if err := s.CreateRun(ctx, Run{ID: "r1", Namespace: "ns", Function: "f", Conversation: "c", Status: StatusResolving, Scopes: []byte("{}")}); err != nil {
			t.Fatal(err)
		}
		at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		for i, n := range []string{"a", "b", "c"} {
			v, _ := Encode(map[string]any{"namespace": "ns", "subject": "lead", "id": "L1", "name": n, "at": at.Add(-time.Duration(i) * time.Hour)})
			if _, err := s.Append(ctx, "r1", Record{Site: n, Kind: KindMilestone, Value: v, At: at}); err != nil {
				t.Fatal(err)
			}
		}
		all, err := s.MilestonesAfter(ctx, "ns", 0, 10)
		if err != nil || len(all) != 3 || all[0].Name != "a" || all[2].Name != "c" {
			t.Fatalf("%s: in the order recorded, not by at: %+v %v", name, all, err)
		}
		if page, _ := s.MilestonesAfter(ctx, "ns", all[0].Seq, 1); len(page) != 1 || page[0].Name != "b" {
			t.Errorf("%s: paging: %+v", name, page)
		}
		if last, _ := s.LastMilestone(ctx, "ns"); last != all[2].Seq {
			t.Errorf("%s: last %d", name, last)
		}
		if _, ok, _ := s.Cursor(ctx, "x"); ok {
			t.Errorf("%s: a new cursor is absent", name)
		}
		_ = s.SetCursor(ctx, "x", 2)
		_ = s.SetCursor(ctx, "x", 5)
		if v, ok, _ := s.Cursor(ctx, "x"); !ok || v != 5 {
			t.Errorf("%s: cursor %d %v", name, v, ok)
		}
	}
}

func TestChanges(t *testing.T) {
	sq, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	for name, s := range map[string]Store{"memory": NewMemory(), "sqlite": sq} {
		ctx := context.Background()
		for _, r := range []Run{{ID: "a", Namespace: "ns"}, {ID: "b", Namespace: "other"}, {ID: "c", Namespace: "ns"}} {
			r.Function, r.Conversation, r.Status, r.Scopes = "f", r.ID, StatusResolving, []byte("{}")
			if err := s.CreateRun(ctx, r); err != nil {
				t.Fatal(err)
			}
		}
		for _, x := range []struct{ run, site string }{{"a", "1"}, {"b", "1"}, {"c", "1"}, {"a", "2"}} {
			if _, err := s.Append(ctx, x.run, Record{Site: x.site, Kind: KindResult, Value: []byte(`{}`), At: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}
		all, err := s.Changes(ctx, "ns", 0, 10)
		if err != nil || len(all) != 3 || all[0].RunID != "a" || all[1].RunID != "c" || all[2].RunID != "a" || all[2].Record.Site != "2" {
			t.Fatalf("%s: in append order, one namespace: %+v %v", name, all, err)
		}
		if rest, _ := s.Changes(ctx, "ns", all[0].Cursor, 1); len(rest) != 1 || rest[0].RunID != "c" {
			t.Errorf("%s: after a cursor: %+v", name, rest)
		}
	}
}

func TestFirstDelivery(t *testing.T) {
	sq, err := OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sq.Close() }()
	for name, s := range map[string]Store{"memory": NewMemory(), "sqlite": sq} {
		ctx := context.Background()
		a, _ := s.FirstDelivery(ctx, "c1", "m1")
		b, _ := s.FirstDelivery(ctx, "c1", "m1")
		c, _ := s.FirstDelivery(ctx, "c2", "m1")
		if !a || b || !c {
			t.Errorf("%s: %v %v %v", name, a, b, c)
		}
	}
}
