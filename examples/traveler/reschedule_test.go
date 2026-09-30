package traveler_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

// agency-008: rescheduling a leg is computed by code, not by the model.

func reschedule(t *testing.T) *compute.Script {
	t.Helper()
	src, err := os.ReadFile("scripts/reschedule.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("reschedule.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func call(t *testing.T, s *compute.Script, fn string, args map[string]any) map[string]any {
	t.Helper()
	n, err := journal.Normalize(args)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Call(context.Background(), fn, n.(map[string]any), compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return out.(map[string]any)
}

func booking(summary string, total int, depart string) map[string]any {
	return map[string]any{"summary": summary, "total": total, "departDate": depart, "locator": "X1"}
}

const roundTrip = "✈️ GOL: ida 03/12 GRU→REC 12:40–17:00, volta 08/12 REC→GRU 12:40–17:00 (direto), 1 adulto — R$ 1.300"

func TestReschedulePlan(t *testing.T) {
	s := reschedule(t)
	p := call(t, s, "plan", map[string]any{"booking": booking(roundTrip, 1300, "2026-12-03"), "leg": "volta", "new_date": "", "shift": 2, "today": "2026-09-28"})
	if p["ok"] != true || p["from"] != "REC" || p["to"] != "GRU" || p["newDate"] != "2026-12-10" || p["paid"] != int64(650) || p["fee"] != int64(150) || p["time"] != "12:40" {
		t.Errorf("the return two days later: %v", p)
	}
	p = call(t, s, "plan", map[string]any{"booking": booking(roundTrip, 1300, "2026-12-03"), "leg": "ida", "new_date": "", "shift": -3, "today": "2026-09-28"})
	if p["newDate"] != "2026-11-30" || p["from"] != "GRU" {
		t.Errorf("across a month, backwards: %v", p)
	}
	for _, c := range []struct {
		name string
		args map[string]any
		says string
	}{
		{"one way has no return", map[string]any{"booking": booking("✈️ GOL: ida 22/11 GRU→POA 12:40–14:15 (direto), 1 adulto — R$ 640", 640, "2026-11-22"), "leg": "volta", "new_date": "2026-11-25", "shift": nil, "today": "2026-09-28"}, "só de ida"},
		{"a past day", map[string]any{"booking": booking(roundTrip, 1300, "2026-12-03"), "leg": "ida", "new_date": "2026-09-01", "shift": nil, "today": "2026-09-28"}, "já passou"},
		{"no day", map[string]any{"booking": booking(roundTrip, 1300, "2026-12-03"), "leg": "ida", "new_date": "", "shift": nil, "today": "2026-09-28"}, "Para qual dia"},
		{"the outbound after the return", map[string]any{"booking": booking(roundTrip, 1300, "2026-12-03"), "leg": "ida", "new_date": "2026-12-09", "shift": nil, "today": "2026-09-28"}, "depois da volta"},
		{"another airline format", map[string]any{"booking": booking("🏨 ibis Natal, 5 noites — R$ 900", 900, "2026-12-01"), "leg": "ida", "new_date": "2026-12-02", "shift": nil, "today": "2026-09-28"}, "equipe"},
	} {
		p := call(t, s, "plan", c.args)
		if p["ok"] != false || !strings.Contains(p["message"].(string), c.says) {
			t.Errorf("%s: %v", c.name, p)
		}
	}
	p = call(t, s, "plan", map[string]any{"booking": booking("✈️ LATAM: ida 16/12 GRU→POA 07:10–08:45, volta 20/12 POA→GRU 07:10–08:45 (direto), 2 adultos, bilhete flexível — R$ 2.480", 2480, "2026-12-16"), "leg": "ida", "new_date": "2026-12-17", "shift": nil, "today": "2026-09-28"})
	if p["fee"] != int64(0) || p["adults"] != int64(2) || p["paid"] != int64(1240) {
		t.Errorf("flexible, two adults: %v", p)
	}
	p = call(t, s, "plan", map[string]any{"booking": booking("✈️ GOL: ida 07/12 GRU→MCZ 12:40–17:00 + 🏨 Maceió Flat Ponta Verde, 4 noites — R$ 2.100", 2100, "2026-12-07"), "leg": "ida", "new_date": "2026-12-08", "shift": nil, "today": "2026-09-28"})
	if p["ok"] != true || p["paid"] != nil {
		t.Errorf("with a hotel the flight's part is unknown: %v", p)
	}
}

func offer(airline, dep string, total int) map[string]any {
	return map[string]any{"priceBreakdown": map[string]any{"total": map[string]any{"units": total}},
		"segments": []any{map[string]any{"departureTime": "2026-12-10T" + dep + ":00", "arrivalTime": "2026-12-10T17:00:00",
			"legs": []any{map[string]any{"carriersData": []any{map[string]any{"name": airline}}}}}}}
}

func TestRescheduleQuote(t *testing.T) {
	s := reschedule(t)
	p := call(t, s, "plan", map[string]any{"booking": booking(roundTrip, 1300, "2026-12-03"), "leg": "volta", "new_date": "", "shift": 2, "today": "2026-09-28"})
	q := call(t, s, "quote", map[string]any{"p": p, "locator": "REC8T1", "offers": []any{offer("LATAM", "07:10", 700), offer("GOL", "19:50", 860), offer("GOL", "12:40", 920)}})
	if q["ok"] != true || q["total"] != int64(420) || !strings.Contains(q["message"].(string), "R$ 920") {
		t.Errorf("the same airline at the booked time; 920 - 650 + 150: %v", q)
	}
	q = call(t, s, "quote", map[string]any{"p": p, "locator": "REC8T1", "offers": []any{offer("GOL", "19:50", 500)}})
	if q["total"] != int64(150) {
		t.Errorf("cheaper: no refund, only the fee: %v", q)
	}
	q = call(t, s, "quote", map[string]any{"p": p, "locator": "REC8T1", "offers": []any{offer("LATAM", "07:10", 700)}})
	if q["ok"] != false || !strings.Contains(q["message"].(string), "GOL") {
		t.Errorf("never another airline: %v", q)
	}
}
