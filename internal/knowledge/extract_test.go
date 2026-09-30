package knowledge

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pdf-001: a PDF's paragraphs and headings come from its layout.
func TestPDFText(t *testing.T) {
	s, err := Text("testdata/convenios.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, "# Convênios aceitos\n\n") || !strings.Contains(s, "\n\n## Carteirinha e autorização\n\n") || !strings.Contains(s, "Amil Dental Básico 100") {
		t.Errorf("pdf text:\n%s", s)
	}
	ps := Split(s)
	if len(ps) != 2 || ps[1].Section != "Convênios aceitos > Carteirinha e autorização" {
		t.Errorf("passages %+v", ps)
	}
	if s, err := Text("testdata/notext.pdf"); err != nil || strings.TrimSpace(s) != "" {
		t.Errorf("an image-only PDF: %q %v", s, err)
	}
	if _, err := Text("testdata/corrupt.pdf"); err == nil {
		t.Errorf("a corrupt PDF was read")
	}
}

// pdf-001: DOCX headings, paragraphs and table rows.
func TestDOCXText(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.docx")
	f, _ := os.Create(file)
	z := zip.NewWriter(f)
	w, _ := z.Create("word/document.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Valores</w:t></w:r></w:p>
<w:p><w:r><w:t xml:space="preserve">Tabela </w:t></w:r><w:r><w:t>vigente.</w:t></w:r></w:p>
<w:p><w:pPr><w:pStyle w:val="Ttulo2"/></w:pPr><w:r><w:t>Limpeza</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Limpeza</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>R$ 180</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
</w:body></w:document>`))
	_ = z.Close()
	_ = f.Close()
	s, err := Text(file)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Valores\n\nTabela vigente.\n\n## Limpeza\n\nLimpeza | R$ 180"
	if s != want {
		t.Errorf("docx text:\n%q\nwant\n%q", s, want)
	}
}
