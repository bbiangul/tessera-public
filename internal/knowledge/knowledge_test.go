package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const doc = `# Convênios aceitos

Atendemos Amil Dental e OdontoPrev.

Não atendemos Unimed Odonto.

## Carteirinha

Traga a carteirinha do plano em toda consulta.
`

func TestSplit(t *testing.T) {
	ps := Split(doc)
	if len(ps) != 2 {
		t.Fatalf("passages: %+v", ps)
	}
	if ps[0].Section != "Convênios aceitos" || !strings.Contains(ps[0].Text, "Amil") || !strings.Contains(ps[0].Text, "Unimed") {
		t.Errorf("first: %+v", ps[0])
	}
	if ps[1].Section != "Convênios aceitos > Carteirinha" || !strings.Contains(ps[1].Text, "carteirinha") {
		t.Errorf("second: %+v", ps[1])
	}
}

func TestQuery(t *testing.T) {
	if q := Query("vcs aceitam a Unimed?", Defaults); q != `"aceitam"* OR "unimed"*` {
		t.Errorf("query %q", q)
	}
	if q := Query("o que é?", Defaults); q != "" {
		t.Errorf("only stopwords: %q", q)
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"convenios.md": doc,
		"interno.md":   "# Descontos\n\nA recepção pode dar até 10% no Pix.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pub, _ := Resolve(filepath.Join(dir, "kb.yaml"), []string{"convenios.md"})
	priv, _ := Resolve(filepath.Join(dir, "kb.yaml"), []string{"interno*.md", "interno.md"})
	if len(pub) != 1 || len(priv) != 1 {
		t.Fatalf("resolve %v %v", pub, priv)
	}
	ix, err := Build([]Base{{Name: "pub", Dir: dir, Files: pub}, {Name: "priv", Dir: dir, Files: priv}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	ps, err := ix.Search("preciso levar a carteirinha?", []string{"pub"}, Defaults)
	if err != nil || len(ps) == 0 || ps[0].Section != "Convênios aceitos > Carteirinha" || ps[0].Source != "convenios.md" || ps[0].Hash == "" {
		t.Fatalf("search: %+v %v", ps, err)
	}
	// A base not asked for is never returned.
	ps, _ = ix.Search("desconto no pix", []string{"pub"}, Defaults)
	for _, p := range ps {
		if p.Base == "priv" {
			t.Errorf("a passage of a base not asked for: %+v", p)
		}
	}
	if ps, _ := ix.Search("desconto no pix", []string{"priv"}, Defaults); len(ps) != 1 || ps[0].Base != "priv" {
		t.Errorf("priv: %+v", ps)
	}
}

// RFC-0008 part C: a lesson's comment is its Meta, never text; expired,
// suspended and retired lessons are not returned.
func TestLessonLifecycle(t *testing.T) {
	doc := "# Lições\n\n## Invisalign?\n\n<!-- lesson id=inv kind=price review=2026-07-01 -->\nA partir de R$ 9.800.\n\n## Sedação?\n\n<!-- lesson id=sed kind=price review=2026-10-20 -->\nR$ 250.\n\n## Crianças?\n\n<!-- lesson id=cri kind=fact review=2027-01-01 status=suspended -->\nA partir de 1 ano.\n"
	dir := t.TempDir()
	f := filepath.Join(dir, "licoes.md")
	if err := os.WriteFile(f, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	ix, err := Build([]Base{{Name: "l", Dir: dir, Files: []string{f}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	o := Defaults
	o.K = 10
	o.Keep = func(p Passage) bool { return Active(p, now) }
	ps, err := ix.Search("invisalign sedação crianças preço", []string{"l"}, o)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range ps {
		ids = append(ids, p.Meta["id"])
		if strings.Contains(p.Text, "lesson") || strings.Contains(p.Text, "review=") {
			t.Errorf("metadata in text: %q", p.Text)
		}
	}
	if len(ids) != 1 || ids[0] != "sed" {
		t.Errorf("active lessons %v", ids)
	}
	// The review day itself still counts; the day after does not.
	sed := Passage{Meta: map[string]string{"review": "2026-10-20"}}
	if !Active(sed, time.Date(2026, 10, 20, 23, 0, 0, 0, time.UTC)) || Active(sed, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("review day boundary")
	}
}
