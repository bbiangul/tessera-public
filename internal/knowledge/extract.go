package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Text reads a document as Markdown-like text: headings as "#", blank
// lines between paragraphs (pdf-001). Markdown and text are read as they
// are; PDF and DOCX are extracted inside the binary.
func Text(file string) (string, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return TextBytes(file, b)
}

// TextBytes is Text for a document already in memory (a plugin source,
// plugin-001); name's extension says its type.
func TextBytes(name string, data []byte) (string, error) {
	r := bytes.NewReader(data)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		return pdfText(filepath.Base(name), r, int64(len(data)))
	case ".docx":
		return docxText(filepath.Base(name), r, int64(len(data)))
	default:
		return string(data), nil
	}
}

// pdfText reads a PDF row by row. PDFs carry no paragraph or heading
// marks the library exposes, so they are inferred from the layout: a gap
// larger than the page's usual line spacing ends a paragraph, and a short
// line alone between such gaps, without final punctuation, is a heading.
func pdfText(file string, ra io.ReaderAt, size int64) (text string, err error) {
	defer func() {
		// The library panics on some malformed files.
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("%s: unreadable PDF: %v", file, r)
		}
	}()
	r, err := pdf.NewReader(ra, size)
	if err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	var blocks [][]string
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		rows, err := p.GetTextByRow()
		if err != nil {
			return "", fmt.Errorf("%s: page %d: %w", file, i, err)
		}
		sort.Slice(rows, func(a, b int) bool { return rows[a].Position < rows[b].Position })
		type line struct {
			y    int64
			text string
		}
		var lines []line
		for _, row := range rows {
			var b strings.Builder
			for _, w := range row.Content {
				b.WriteString(w.S)
			}
			if t := strings.TrimSpace(b.String()); t != "" {
				lines = append(lines, line{row.Position, t})
			}
		}
		if len(lines) == 0 {
			continue
		}
		var gaps []int64
		for j := 1; j < len(lines); j++ {
			if g := lines[j].y - lines[j-1].y; g > 0 {
				gaps = append(gaps, g)
			}
		}
		spacing := int64(0)
		if len(gaps) > 0 {
			sort.Slice(gaps, func(a, b int) bool { return gaps[a] < gaps[b] })
			spacing = gaps[len(gaps)/2]
		}
		cur := []string{lines[0].text}
		for j := 1; j < len(lines); j++ {
			if spacing > 0 && lines[j].y-lines[j-1].y > spacing*14/10 {
				blocks = append(blocks, cur)
				cur = nil
			}
			cur = append(cur, lines[j].text)
		}
		blocks = append(blocks, cur)
	}
	var out []string
	for i, b := range blocks {
		t := strings.Join(b, " ")
		if len(b) == 1 && headingLike(t) {
			level := "## "
			if i == 0 {
				level = "# "
			}
			out = append(out, level+t)
			continue
		}
		out = append(out, t)
	}
	return strings.Join(out, "\n\n"), nil
}

// headingLike: short, and no sentence ending.
func headingLike(s string) bool {
	if len(strings.Fields(s)) > 12 {
		return false
	}
	return !strings.ContainsAny(s[len(s)-1:], ".,;:!?")
}

var headingStyle = regexp.MustCompile(`(?i)^(heading|t[ií]?tulo)\s*(\d)$`)

// docxText reads word/document.xml: heading styles become "#" headings,
// paragraphs stay paragraphs, and table rows become "cell | cell" lines.
func docxText(file string, ra io.ReaderAt, size int64) (string, error) {
	z, err := zip.NewReader(ra, size)
	if err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	var doc io.ReadCloser
	for _, f := range z.File {
		if f.Name == "word/document.xml" {
			if doc, err = f.Open(); err != nil {
				return "", err
			}
			break
		}
	}
	if doc == nil {
		return "", fmt.Errorf("%s: no word/document.xml", file)
	}
	defer func() { _ = doc.Close() }()
	dec := xml.NewDecoder(doc)
	var out []string
	var para strings.Builder
	var style string
	var inText bool
	var tableDepth int
	var cells []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%s: %w", file, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				para.Reset()
				style = ""
			case "pStyle":
				for _, a := range t.Attr {
					if a.Name.Local == "val" {
						style = a.Value
					}
				}
			case "t":
				inText = true
			case "tab":
				para.WriteString(" ")
			case "br":
				para.WriteString(" ")
			case "tbl":
				tableDepth++
			case "tr":
				cells = nil
			}
		case xml.CharData:
			if inText {
				para.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				text := strings.TrimSpace(para.String())
				if text == "" {
					continue
				}
				if tableDepth > 0 {
					cells = append(cells, text)
					continue
				}
				if m := headingStyle.FindStringSubmatch(style); m != nil {
					n, _ := strconv.Atoi(m[2])
					if n < 1 {
						n = 1
					}
					text = strings.Repeat("#", min(n, 6)) + " " + text
				}
				out = append(out, text)
			case "tr":
				if len(cells) > 0 {
					out = append(out, strings.Join(cells, " | "))
				}
				cells = nil
			case "tbl":
				tableDepth--
			}
		}
	}
	return strings.Join(out, "\n\n"), nil
}
