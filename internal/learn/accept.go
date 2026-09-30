package learn

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Validity is how long a lesson holds before a person reviews it again
// (RFC-0008, decided 2026-09-26).
var Validity = map[string]time.Duration{
	"price":  30 * 24 * time.Hour,
	"policy": 180 * 24 * time.Hour,
	"fact":   365 * 24 * time.Hour,
}

var lessonID = regexp.MustCompile(`<!-- lesson id=([^ ]+) `)

// Accept appends the proposals whose ids are given to a lessons file (a
// KnowledgeBase document): one heading per lesson, its provenance in an
// HTML comment the index skips. A lesson already in the file is not
// added again. It returns the ids written.
func Accept(file string, proposals []Proposal, ids []string, now time.Time) ([]string, error) {
	return AcceptSuperseding(file, proposals, ids, nil, now)
}

// AcceptSuperseding accepts proposals that replace lessons of the file:
// the replaced ones are retired (superseded, by the new ids). A new
// lesson on the same question as the one it replaces gets its own id
// (the date appended). Nothing is retired unless something was written.
func AcceptSuperseding(file string, proposals []Proposal, ids, supersede []string, now time.Time) ([]string, error) {
	raw, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	have := map[string]bool{}
	for _, m := range lessonID.FindAllStringSubmatch(string(raw), -1) {
		have[m[1]] = true
	}
	byID := map[string]Proposal{}
	for _, p := range proposals {
		byID[p.ID] = p
	}
	var b strings.Builder
	var wrote []string
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("no proposal %q", id)
		}
		if have[id] {
			if !slices.Contains(supersede, id) {
				continue
			}
			p.ID = id + "-" + now.Format("20060102")
			if have[p.ID] {
				continue
			}
		}
		id := p.ID
		var runs []string
		for _, c := range p.Evidence {
			runs = append(runs, c.Run)
		}
		review := now.Add(Validity[p.Kind]).Format("2006-01-02")
		fmt.Fprintf(&b, "\n## %s\n\n<!-- lesson id=%s kind=%s learned=%s review=%s cases=%d members=%s runs=%s -->\n%s\n",
			oneLine(p.Question), id, p.Kind, now.Format("2006-01-02"), review, len(p.Evidence), strings.Join(p.Members, ","), strings.Join(runs, ","), strings.TrimSpace(p.Fact))
		wrote = append(wrote, id)
	}
	if len(wrote) == 0 {
		if len(supersede) > 0 {
			return nil, fmt.Errorf("nothing was written, so nothing is superseded")
		}
		return nil, nil
	}
	out := string(raw)
	if out == "" {
		out = "# Lições aprendidas\n\nRespostas da equipe aprovadas como lições (tessera learn).\n"
	}
	if err := os.WriteFile(file, []byte(strings.TrimRight(out, "\n")+"\n"+b.String()), 0o644); err != nil {
		return nil, err
	}
	if len(supersede) > 0 {
		if _, err := SetMeta(file, supersede, map[string]string{"status": "retired", "reason": "superseded", "by": strings.Join(wrote, ","), "on": now.Format("2006-01-02")}); err != nil {
			return wrote, err
		}
	}
	return wrote, nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
