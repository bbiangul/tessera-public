// Command recall measures knowledge-001's retrieval: for each question
// with a gold document, whether a passage of it is among the top k.
//
//	go run ./bench/knowledge/recall -questions bench/knowledge/questions-dev.json -grid
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bbiangul/tessera/internal/engines/generate"
	"github.com/bbiangul/tessera/internal/knowledge"
)

type question struct {
	ID      string `json:"id"`
	Doc     string `json:"doc"`
	Message string `json:"message"`
}

func main() {
	qfile := flag.String("questions", "bench/knowledge/questions-dev.json", "questions with a gold doc")
	docs := flag.String("docs", "bench/knowledge/docs", "documents directory")
	grid := flag.Bool("grid", false, "every combination of the options, k 2..6")
	glob := flag.String("glob", "*.md", "documents to index (pdf-001: *.pdf, *.docx)")
	show := flag.Bool("show", false, "print the misses of the default options")
	rewrite := flag.String("rewrite", "", "also search the Generate engine's words (RewriteRules), e.g. deepseek/deepseek-v4.1-flash; needs OPENROUTER_API_KEY")
	flag.Parse()
	var qs []question
	raw, err := os.ReadFile(*qfile)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(raw, &qs); err != nil {
		log.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(*docs, *glob))
	ix, err := knowledge.Build([]knowledge.Base{{Name: "kb", Dir: *docs, Files: files}})
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	extra := map[string]string{}
	if *rewrite != "" {
		or := &generate.OpenRouter{APIKey: os.Getenv("OPENROUTER_API_KEY")}
		or.Model, or.Providers, _ = generate.ParseModel(*rewrite)
		for _, q := range qs {
			r, err := or.Generate(context.Background(), generate.Request{Instructions: knowledge.RewriteRules, State: map[string]any{"message": q.Message}, MaxTokens: 60})
			if err != nil {
				log.Fatal(err)
			}
			extra[q.ID] = r.Text
			fmt.Printf("  %s: %q -> %q\n", q.ID, q.Message, r.Text)
		}
	}
	run := func(o knowledge.Options, print bool) (hit, n int) {
		for _, q := range qs {
			if q.Doc == "" {
				continue
			}
			n++
			ps, err := ix.Search(q.Message+" "+extra[q.ID], []string{"kb"}, o)
			if err != nil {
				log.Fatal(err)
			}
			found := false
			for _, p := range ps {
				found = found || strings.TrimSuffix(p.Source, filepath.Ext(p.Source)) == strings.TrimSuffix(q.Doc, filepath.Ext(q.Doc))
			}
			if found {
				hit++
			} else if print {
				var got []string
				for _, p := range ps {
					got = append(got, p.Source)
				}
				fmt.Printf("  miss %s %q want %s got %v\n", q.ID, q.Message, q.Doc, got)
			}
		}
		return hit, n
	}
	if *grid {
		for _, stop := range []bool{false, true} {
			for _, prefix := range []bool{false, true} {
				for _, all := range []bool{false, true} {
					for k := 2; k <= 6; k += 2 {
						o := knowledge.Options{K: k, Stopwords: stop, Prefix: prefix, All: all}
						h, n := run(o, false)
						fmt.Printf("stop=%v prefix=%v and=%v k=%d: %d/%d\n", stop, prefix, all, k, h, n)
					}
				}
			}
		}
	}
	h, n := run(knowledge.Defaults, *show)
	fmt.Printf("defaults %+v: recall@%d %d/%d (%d passages)\n", knowledge.Defaults, knowledge.Defaults.K, h, n, ix.Len())
}
