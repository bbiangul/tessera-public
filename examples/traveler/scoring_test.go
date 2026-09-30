// Package traveler_test checks the traveler agent's Starlark against the
// the legacy platform originals it replaces (RFC-0001, traveler acceptance criteria).
package traveler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bbiangul/tessera/internal/engines/compute"
	"github.com/bbiangul/tessera/internal/runtime/journal"
)

func script(t *testing.T) *compute.Script {
	t.Helper()
	src, err := os.ReadFile("scripts/scoring.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("scoring.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// fixture holds one parity case: the inputs the original received and what
// it printed (recorded with the legacy parity script, not included).
type fixture struct {
	RawFlights struct {
		Data struct {
			FlightOffers []any `json:"flightOffers"`
		} `json:"data"`
	} `json:"rawFlights"`
	UserPrefs      map[string]any `json:"userPrefs"`
	TimeConstraint string         `json:"timeConstraint"`
}

func load(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// intendedDifferences are the fixes the port makes on purpose; everything
// else must match the original exactly.
var intendedDifferences = map[string]func(legacy map[string]any){
	// The original compared "gol" with IATA codes and filtered nothing;
	// the port filters GOL's flights (G3).
	"avoid-name": func(legacy map[string]any) {
		var kept []any
		for _, f := range legacy["topFlights"].([]any) {
			codes := f.(map[string]any)["airlines"].([]any)
			if !contains(codes, "G3") {
				kept = append(kept, f)
			}
		}
		legacy["topFlights"] = kept
	},
}

func contains(xs []any, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestScoreFlightsMatchesTheOriginal(t *testing.T) {
	s := script(t)
	inputs, _ := filepath.Glob("testdata/score_flights/*.input.json")
	if len(inputs) < 10 {
		t.Fatalf("found %d parity fixtures", len(inputs))
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".input.json")
		t.Run(name, func(t *testing.T) {
			var fx fixture
			load(t, in, &fx)
			var legacy map[string]any
			load(t, strings.TrimSuffix(in, ".input.json")+".legacy.json", &legacy)

			args := map[string]any{"offers": fx.RawFlights.Data.FlightOffers, "prefs": fx.UserPrefs, "time_text": fx.TimeConstraint}
			norm, err := journal.Normalize(args) // json.Number -> int64/float64
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Call(context.Background(), "score_flights", norm.(map[string]any), compute.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			gotNorm, _ := journal.Normalize(got)
			legacyNorm, _ := journal.Normalize(legacy)
			lm := legacyNorm.(map[string]any)
			// Fix 2: the original's loyaltyProgram ended with a newline.
			lm["loyaltyProgram"] = strings.TrimSuffix(lm["loyaltyProgram"].(string), "\n")
			if fix, ok := intendedDifferences[name]; ok {
				fix(lm)
				// Summary counts follow the filtered list; compare the flights only.
				delete(lm, "summary")
				delete(gotNorm.(map[string]any), "summary")
				top := gotNorm.(map[string]any)["topFlights"].([]any)
				lm["topFlights"] = lm["topFlights"].([]any)[:min(len(top), len(lm["topFlights"].([]any)))]
				gotNorm.(map[string]any)["topFlights"] = top[:len(lm["topFlights"].([]any))]
			}
			var diffs []string
			diff("", lm, gotNorm, &diffs)
			if len(diffs) > 0 {
				sort.Strings(diffs)
				if len(diffs) > 15 {
					diffs = append(diffs[:15], fmt.Sprintf("… and %d more", len(diffs)-15))
				}
				t.Errorf("differs from the original:\n  %s", strings.Join(diffs, "\n  "))
			}
		})
	}
}

// diff lists paths where two JSON-like values differ; numbers are equal
// within 1e-9 (jq prints doubles).
func diff(path string, want, got any, out *[]string) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: want an object, got %T", path, got))
			return
		}
		keys := map[string]bool{}
		for k := range w {
			keys[k] = true
		}
		for k := range g {
			keys[k] = true
		}
		for k := range keys {
			diff(path+"."+k, w[k], g[k], out)
		}
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) != len(w) {
			*out = append(*out, fmt.Sprintf("%s: want %d items, got %v", path, len(w), describe(got)))
			return
		}
		for i := range w {
			diff(fmt.Sprintf("%s[%d]", path, i), w[i], g[i], out)
		}
	default:
		wf, wok := num(want)
		gf, gok := num(got)
		if wok && gok {
			if math.Abs(wf-gf) > 1e-9 {
				*out = append(*out, fmt.Sprintf("%s: want %v, got %v", path, want, got))
			}
			return
		}
		if fmt.Sprint(want) != fmt.Sprint(got) {
			*out = append(*out, fmt.Sprintf("%s: want %#v, got %#v", path, want, got))
		}
	}
}

func num(v any) (float64, bool) {
	switch n := v.(type) {
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func describe(v any) string {
	if l, ok := v.([]any); ok {
		return fmt.Sprintf("%d items", len(l))
	}
	return fmt.Sprintf("%T", v)
}

func TestScoreFlightsWhenEverythingIsFiltered(t *testing.T) {
	// The original printed invalid JSON here (fix 3).
	var fx fixture
	load(t, "testdata/score_flights/balanced.input.json", &fx)
	norm, _ := journal.Normalize(map[string]any{"offers": fx.RawFlights.Data.FlightOffers, "prefs": map[string]any{}, "time_text": "chegar até 1h"})
	got, err := script(t).Call(context.Background(), "score_flights", norm.(map[string]any), compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if len(m["topFlights"].([]any)) != 0 || m["summary"].(map[string]any)["hasInfants"] != false {
		t.Errorf("got %v", m)
	}
}

func TestTimeConstraintParsing(t *testing.T) {
	s := script(t)
	for text, want := range map[string]map[string]any{
		"chegar até 15h":         {"arriveBy": "15:00"},
		"arrive by 9":            {"arriveBy": "9:00"},
		"sair depois das 10h30":  {"departAfter": "10:0030"}, // as the original; minutes still parse to 10:30
		"de manhã, sem pernoite": {"preference": "morning", "noOvernight": true},
		"à tarde":                {"preference": "afternoon"},
		"voo de madrugada":       {"preference": "night"},
	} {
		got, err := s.Call(context.Background(), "time_constraint", map[string]any{"text": text}, compute.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range want {
			if got.(map[string]any)[k] != v {
				t.Errorf("%q: %s = %v, want %v", text, k, got.(map[string]any)[k], v)
			}
		}
	}
}

// hotelFixture is one score_hotels parity case.
type hotelFixture struct {
	RawHotels struct {
		Data struct {
			Hotels []any `json:"hotels"`
		} `json:"data"`
	} `json:"rawHotels"`
	UserPreferences map[string]any `json:"userPreferences"`
	PropertyType    struct {
		PropertyType string `json:"propertyType"`
	} `json:"propertyType"`
}

// applyHotelFixes applies the port's intended fixes to the original's
// output wherever they apply, so the comparison stays exact elsewhere.
func applyHotelFixes(legacy map[string]any, input hotelFixture) {
	// Fix 1: serviced_apartment gets the apartment bonus.
	if input.PropertyType.PropertyType == "serviced_apartment" {
		rescore(legacy, input, func(h map[string]any) float64 {
			if strings.Contains(strings.ToLower(h["propertyType"].(string)), "apart") {
				return 15
			}
			return 0
		})
	}
	// Fix 2: in price mode, no price bonus for a hotel without a price.
	if input.UserPreferences["time_value"] == "price" {
		noPrice := map[string]bool{}
		for _, h := range input.RawHotels.Data.Hotels {
			hm := h.(map[string]any)
			if _, ok := hm["property"].(map[string]any)["priceBreakdown"]; !ok {
				noPrice[fmt.Sprint(hm["hotel_id"])] = true
			}
		}
		rescore(legacy, input, func(h map[string]any) float64 {
			if noPrice[h["hotelId"].(string)] {
				return -15 // the original gave +15 for a price of 0
			}
			return 0
		})
	}
}

// rescore adds delta to each hotel's adjustedScore and sorts again the way
// the original's stable sort_by would on the fixed scores: by score, ties
// in input order.
func rescore(legacy map[string]any, input hotelFixture, delta func(map[string]any) float64) {
	pos := map[string]int{}
	for i, h := range input.RawHotels.Data.Hotels {
		pos[fmt.Sprint(h.(map[string]any)["hotel_id"])] = i
	}
	top := legacy["topHotels"].([]any)
	for _, h := range top {
		hm := h.(map[string]any)
		s, _ := num(hm["adjustedScore"])
		hm["adjustedScore"] = int64(s + delta(hm))
	}
	sort.SliceStable(top, func(i, j int) bool {
		a, _ := num(top[i].(map[string]any)["adjustedScore"])
		b, _ := num(top[j].(map[string]any)["adjustedScore"])
		if a != b {
			return a > b
		}
		return pos[top[i].(map[string]any)["hotelId"].(string)] < pos[top[j].(map[string]any)["hotelId"].(string)]
	})
}

func TestScoreHotelsMatchesTheOriginal(t *testing.T) {
	src, err := os.ReadFile("scripts/hotels.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("hotels.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := filepath.Glob("testdata/score_hotels/*.input.json")
	if len(inputs) < 5 {
		t.Fatalf("found %d parity fixtures", len(inputs))
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".input.json")
		t.Run(name, func(t *testing.T) {
			var fx hotelFixture
			load(t, in, &fx)
			var legacy map[string]any
			load(t, strings.TrimSuffix(in, ".input.json")+".legacy.json", &legacy)
			args, err := journal.Normalize(map[string]any{"hotels": fx.RawHotels.Data.Hotels, "prefs": fx.UserPreferences, "property_type": fx.PropertyType.PropertyType})
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Call(context.Background(), "score_hotels", args.(map[string]any), compute.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			gotNorm, _ := journal.Normalize(got)
			legacyNorm, _ := journal.Normalize(legacy)
			lm := legacyNorm.(map[string]any)
			applyHotelFixes(lm, fx)
			var diffs []string
			diff("", lm, gotNorm, &diffs)
			if len(diffs) > 0 {
				sort.Strings(diffs)
				t.Errorf("differs from the original:\n  %s", strings.Join(diffs, "\n  "))
			}
		})
	}
}

func TestSeatAnalysisMatchesTheOriginal(t *testing.T) {
	src, err := os.ReadFile("scripts/seats.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("seats.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := filepath.Glob("testdata/seats/*.input.json")
	if len(inputs) < 4 {
		t.Fatalf("found %d parity fixtures", len(inputs))
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".input.json")
		t.Run(name, func(t *testing.T) {
			var fx map[string]any
			load(t, in, &fx)
			var legacy []any
			load(t, strings.TrimSuffix(in, ".input.json")+".legacy.json", &legacy)
			args, err := journal.Normalize(map[string]any{"scored": fx["scoredFlights"], "seat_maps": fx["seatMaps"]})
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Call(context.Background(), "analyze_seats", args.(map[string]any), compute.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			gotNorm, _ := journal.Normalize(got)
			legacyNorm, _ := journal.Normalize(legacy)
			if name == "infant-mid" {
				// The intended fix: row 12 has two bassinet seats; the
				// original counted the row twice (2 seats x 2 = 4).
				entry := legacyNorm.([]any)[0].(map[string]any)
				if entry["bassinetSeatsCount"] != int64(4) {
					t.Fatalf("the fixture no longer shows the original's double count: %v", entry["bassinetSeatsCount"])
				}
				entry["bassinetSeatsCount"] = int64(2)
			}
			var diffs []string
			diff("", legacyNorm, gotNorm, &diffs)
			if len(diffs) > 0 {
				sort.Strings(diffs)
				t.Errorf("differs from the original:\n  %s", strings.Join(diffs, "\n  "))
			}
		})
	}
}

func hotels(t *testing.T) *compute.Script {
	t.Helper()
	src, err := os.ReadFile("scripts/hotels.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("hotels.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The original's rule (08_hotel_analysis): up to 3 nights a hotel, up to 7
// either, up to 14 an aparthotel, longer a serviced apartment; what the
// user asked for wins. Nights across a month and a DST change.
func TestPropertyTypeByLengthOfStay(t *testing.T) {
	s := hotels(t)
	for _, c := range []struct {
		in, out, wanted, kind string
		nights                int64
	}{
		{"2026-10-12", "2026-10-15", "", "hotel", 3},
		{"2026-10-12", "2026-10-16", "", "flexible", 4},
		{"2026-10-12", "2026-10-19", "", "flexible", 7},
		{"2026-10-12", "2026-10-20", "", "aparthotel", 8},
		{"2026-10-25", "2026-11-08", "", "aparthotel", 14},
		{"2026-10-25", "2026-11-09", "", "serviced_apartment", 15},
		{"2026-10-12", "2026-10-13", "apartment", "apartment", 1},
		{"2026-10-12", "2026-11-12", "hotel", "hotel", 31},
	} {
		got, err := s.Call(context.Background(), "property_type", map[string]any{"check_in": c.in, "check_out": c.out, "wanted": c.wanted}, compute.Limits{})
		m, _ := got.(map[string]any)
		if err != nil || m["kind"] != c.kind || m["nights"] != c.nights {
			t.Errorf("%s..%s %q: %v %v, want %s for %d nights", c.in, c.out, c.wanted, got, err, c.kind, c.nights)
		}
	}
}

// The original's resolveDestination: one result wins, then the first city,
// then the first result; none fails with a code the Function explains.
func TestPickDestination(t *testing.T) {
	s := hotels(t)
	place := func(id, typ string) map[string]any {
		return map[string]any{"dest_id": id, "dest_type": typ, "name": id}
	}
	for _, c := range []struct {
		places []any
		want   string
	}{
		{[]any{place("d1", "district")}, "d1"},
		{[]any{place("d1", "district"), place("c1", "city"), place("c2", "city")}, "c1"},
		{[]any{place("l1", "landmark"), place("d1", "district")}, "l1"},
	} {
		got, err := s.Call(context.Background(), "pick_destination", map[string]any{"places": c.places}, compute.Limits{})
		if m, _ := got.(map[string]any); err != nil || m["destId"] != c.want {
			t.Errorf("%v: %v %v, want %s", c.places, got, err, c.want)
		}
	}
	if _, err := s.Call(context.Background(), "pick_destination", map[string]any{"places": []any{}}, compute.Limits{}); err == nil || !strings.Contains(err.Error(), "NO_DESTINATION") {
		t.Errorf("no place must fail with NO_DESTINATION: %v", err)
	}
}

func connections(t *testing.T) *compute.Script {
	t.Helper()
	src, err := os.ReadFile("scripts/connections.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("connections.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// parity runs one Starlark function on every fixture of a directory and
// compares it with what the original printed. Where the original left
// fields out (nothing to report), only its fields are compared: the port
// always answers in one shape (intended difference 1); warning texts are
// the message's job now (difference 2).
func parity(t *testing.T, dir, function string, args func(in map[string]any) map[string]any, fix func(in, legacy map[string]any)) {
	s := connections(t)
	inputs, _ := filepath.Glob("testdata/" + dir + "/*.input.json")
	if len(inputs) < 4 {
		t.Fatalf("found %d parity fixtures", len(inputs))
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".input.json")
		t.Run(name, func(t *testing.T) {
			var fx, legacy map[string]any
			load(t, in, &fx)
			load(t, strings.TrimSuffix(in, ".input.json")+".legacy.json", &legacy)
			norm, err := journal.Normalize(args(fx))
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Call(context.Background(), function, norm.(map[string]any), compute.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			gotNorm, _ := journal.Normalize(got)
			if fix != nil {
				fix(fx, legacy)
			}
			lm, _ := journal.Normalize(legacy)
			gm := gotNorm.(map[string]any)
			for k := range gm {
				if _, ok := lm.(map[string]any)[k]; !ok {
					delete(gm, k)
				}
			}
			if sum, ok := lm.(map[string]any)["summary"].(map[string]any); ok {
				for k := range gm["summary"].(map[string]any) {
					if _, ok := sum[k]; !ok {
						delete(gm["summary"].(map[string]any), k)
					}
				}
			}
			if ras, ok := lm.(map[string]any)["riskAssessments"].([]any); ok {
				for _, a := range ras {
					delete(a.(map[string]any), "warning")
				}
			}
			var diffs []string
			diff("", lm, gm, &diffs)
			if len(diffs) > 0 {
				sort.Strings(diffs)
				t.Errorf("differs from the original:\n  %s", strings.Join(diffs, "\n  "))
			}
		})
	}
}

func TestAirportChangesMatchTheOriginal(t *testing.T) {
	parity(t, "airport_changes", "airport_changes", func(in map[string]any) map[string]any {
		raw := in["rawFlights"].(map[string]any)
		data, _ := raw["data"].(map[string]any)
		return map[string]any{"offers": data["flightOffers"]}
	}, nil)
}

func TestAirportChangeRiskMatchesTheOriginal(t *testing.T) {
	parity(t, "airport_risk", "assess", func(in map[string]any) map[string]any {
		return map[string]any{"detected": in["airportChangeData"], "transfers": in["transferTimeData"]}
	}, func(in, legacy map[string]any) {
		// Fix 3: estimated is true only when no measured time exists.
		measured := map[string]bool{}
		for _, x := range in["transferTimeData"].(map[string]any)["transferTimes"].([]any) {
			m := x.(map[string]any)
			if m["estimated"] == false {
				measured[m["from"].(string)+m["to"].(string)] = true
			}
		}
		ras, _ := legacy["riskAssessments"].([]any)
		for _, a := range ras {
			m := a.(map[string]any)
			m["estimated"] = !measured[m["fromAirport"].(string)+m["toAirport"].(string)]
		}
	})
}

// The route matrix is asked only for pairs with coordinates, each airport
// once; its answer maps back to pairs, and what it did not answer is
// estimated. The matrix does not exist in the original (one call per pair).
func TestRouteMatrixRoundTrip(t *testing.T) {
	s := connections(t)
	ctx := context.Background()
	detected := map[string]any{"uniquePairs": []any{
		map[string]any{"from": "GRU", "to": "CGH"}, map[string]any{"from": "GRU", "to": "VCP"},
		map[string]any{"from": "LHR", "to": "LGW"}, map[string]any{"from": "BBB", "to": "CCC"},
	}}
	req, err := s.Call(ctx, "matrix_request", map[string]any{"detected": detected}, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	r := req.(map[string]any)
	if len(r["origins"].([]any)) != 2 || len(r["destinations"].([]any)) != 3 || len(r["cells"].([]any)) != 3 {
		t.Fatalf("request %v", r)
	}
	elements := []any{
		map[string]any{"originIndex": int64(0), "destinationIndex": int64(0), "duration": "4470s", "condition": "ROUTE_EXISTS"},
		map[string]any{"originIndex": int64(0), "destinationIndex": int64(1), "condition": "ROUTE_NOT_FOUND"},
		map[string]any{"originIndex": int64(1), "destinationIndex": int64(0), "duration": "3000s"}, // LHR→CGH: not asked for
	}
	got, err := s.Call(ctx, "transfer_times", map[string]any{"request": r, "elements": elements}, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GRU→CGH 74 false", "GRU→VCP 90 true", "LHR→LGW 90 true"}
	var have []string
	for _, x := range got.(map[string]any)["transferTimes"].([]any) {
		m := x.(map[string]any)
		have = append(have, fmt.Sprintf("%s→%s %v %v", m["from"], m["to"], m["transferMinutes"], m["estimated"]))
	}
	if strings.Join(have, ", ") != strings.Join(want, ", ") {
		t.Errorf("transfers %v, want %v", have, want)
	}
}

// The original's merge: a flight's worst change costs 25 points (HIGH) or
// 15 (MEDIUM) and the list is sorted again; a flight without a change is
// untouched.
func TestWithRisk(t *testing.T) {
	s := connections(t)
	ranked := map[string]any{"summary": map[string]any{"currency": "BRL"}, "topFlights": []any{
		map[string]any{"token": "cheap", "score": int64(110)},
		map[string]any{"token": "direct", "score": int64(100)},
		map[string]any{"token": "ok", "score": int64(95)},
	}}
	risk := map[string]any{"riskAssessments": []any{
		map[string]any{"flightToken": "cheap", "fromAirport": "GRU", "toAirport": "CGH", "layoverMinutes": int64(300), "transferMinutes": int64(74), "marginMinutes": int64(226), "riskLevel": "LOW"},
		map[string]any{"flightToken": "cheap", "fromAirport": "SDU", "toAirport": "GIG", "layoverMinutes": int64(60), "transferMinutes": int64(40), "marginMinutes": int64(20), "riskLevel": "HIGH"},
		map[string]any{"flightToken": "ok", "fromAirport": "GRU", "toAirport": "CGH", "layoverMinutes": int64(200), "transferMinutes": int64(74), "marginMinutes": int64(126), "riskLevel": "LOW"},
	}}
	got, err := s.Call(context.Background(), "with_risk", map[string]any{"ranked": ranked, "risk": risk}, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, x := range got.(map[string]any)["topFlights"].([]any) {
		f := x.(map[string]any)
		order = append(order, fmt.Sprintf("%s %v %v %v", f["token"], f["score"], f["airportChangeRisk"], len(f["airportChanges"].([]any))))
	}
	want := "direct 100 <nil> 0, ok 95 LOW 1, cheap 85 HIGH 2"
	if strings.Join(order, ", ") != want {
		t.Errorf("got %s\nwant %s", strings.Join(order, ", "), want)
	}
}

// determineCurrency: the original printed the code as text. The cases
// named star-* differ on purpose (scripts/currency.star).
func TestCurrencyMatchesTheOriginal(t *testing.T) {
	src, err := os.ReadFile("scripts/currency.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("currency.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	intended := map[string]string{"star-pernambuco": "BRL", "star-atlantis": "BRL", "star-londres-roundtrip": "GBP"}
	inputs, _ := filepath.Glob("testdata/currency/*.input.json")
	if len(inputs) < 12 {
		t.Fatalf("found %d parity fixtures", len(inputs))
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".input.json")
		t.Run(name, func(t *testing.T) {
			var fx struct{ HomeCity, OriginCity, IsRoundTrip string }
			load(t, in, &fx)
			raw, err := os.ReadFile(strings.TrimSuffix(in, ".input.json") + ".legacy.txt")
			if err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSpace(string(raw))
			if w, ok := intended[name]; ok {
				if w == want {
					t.Fatalf("listed as an intended difference, but the original already says %s", want)
				}
				want = w
			}
			got, err := s.Call(context.Background(), "currency", map[string]any{
				"home_city": fx.HomeCity, "origin": fx.OriginCity, "round_trip": fx.IsRoundTrip != "",
			}, compute.Limits{})
			if err != nil || got != want {
				t.Errorf("got %v %v, want %s", got, err, want)
			}
		})
	}
}

// Prices of extras are read by code: units plus nanos, rounded; pounds
// become kilos; what the API does not offer is left out.
func TestExtrasMessage(t *testing.T) {
	src, err := os.ReadFile("scripts/extras.star")
	if err != nil {
		t.Fatal(err)
	}
	s, err := compute.Compile("extras.star", src, compute.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	price := func(units, nanos int64) map[string]any {
		return map[string]any{"total": map[string]any{"currencyCode": "BRL", "units": units, "nanos": nanos}}
	}
	flight := map[string]any{"label": "2. Azul GRU → REC", "baggage": "Tarifa Light: bagagem de mão; mala despachada não inclusa."}
	full := map[string]any{
		"checkedInBaggage": map[string]any{"options": []any{map[string]any{
			"luggageAllowance": map[string]any{"maxPiece": int64(1), "maxWeightPerPiece": 50.7, "massUnit": "LB"},
			"priceBreakdown":   price(1409, 910000000)}}},
		"flexibleTicket":  map[string]any{"priceBreakdown": price(472, 900000000)},
		"travelInsurance": map[string]any{"options": map[string]any{"priceBreakdown": price(308, 310000000)}, "content": map[string]any{"benefits": []any{"Trip cancellation", "Baggage loss"}}},
	}
	for _, c := range []struct {
		data any
		want []string
	}{
		{full, []string{"Mala despachada (1 peça de até 23 kg): R$ 1.410", "Bilhete flexível (remarcar ou cancelar): R$ 473", "Seguro viagem: R$ 308 — cobre trip cancellation; baggage loss", "Incluso: Tarifa Light"}},
		{map[string]any{}, []string{"não oferece extras"}},
		{nil, []string{"não oferece extras"}},
	} {
		got, err := s.Call(context.Background(), "extras_message", map[string]any{"flight": flight, "data": c.data}, compute.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range c.want {
			if !strings.Contains(got.(string), w) {
				t.Errorf("%v: message lacks %q:\n%s", c.data, w, got)
			}
		}
	}
}
