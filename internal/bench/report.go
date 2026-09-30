package bench

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// WriteMarkdown writes a comparison report for the given summaries.
func WriteMarkdown(w io.Writer, title string, dataset string, cases, repeats int, sums []Summary) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "Dataset `%s`: %d cases × %d repeats. Every target received the same question text.\n\n", dataset, cases, repeats)

	b.WriteString("| Target | Case acc. (95% CI) | Trial acc. | Consistency | Brier ↓ | Log loss ↓ | ECE ↓ | AUROC ↑ | p50 | p95 | $/question | Errors | Prob. source |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, s := range sums {
		fmt.Fprintf(&b, "| %s | %.1f%% (%.1f–%.1f) | %.1f%% | %.0f%% | %.3f | %.3f | %.3f | %s | %s | %s | %.7f | %d | %s |\n",
			s.Target, 100*s.CaseAccuracy, 100*s.CaseCI[0], 100*s.CaseCI[1], 100*s.TrialAccuracy,
			100*s.Consistency, s.Brier, s.LogLoss, s.ECE, auroc(s.AUROC), ms(s.LatencyP50), ms(s.LatencyP95),
			s.CostPerQ, s.Errors, sources(s))
	}

	b.WriteString("\n## Selective accuracy (act only when top probability ≥ threshold)\n\n")
	b.WriteString("| Target | ≥0.5 cov / acc | ≥0.7 cov / acc | ≥0.9 cov / acc |\n| --- | --- | --- | --- |\n")
	for _, s := range sums {
		fmt.Fprintf(&b, "| %s", s.Target)
		for _, sel := range s.Select {
			fmt.Fprintf(&b, " | %.0f%% / %.1f%%", 100*sel.Coverage, 100*sel.Accuracy)
		}
		b.WriteString(" |\n")
	}

	b.WriteString("\n## Accuracy by task (trials)\n\n")
	tasks := map[string]bool{}
	for _, s := range sums {
		for t := range s.ByTask {
			tasks[t] = true
		}
	}
	names := make([]string, 0, len(tasks))
	for t := range tasks {
		names = append(names, t)
	}
	sort.Strings(names)
	b.WriteString("| Target | " + strings.Join(names, " | ") + " |\n|---" + strings.Repeat("|---", len(names)) + "|\n")
	for _, s := range sums {
		fmt.Fprintf(&b, "| %s", s.Target)
		for _, t := range names {
			fmt.Fprintf(&b, " | %.0f%%", 100*s.ByTask[t])
		}
		b.WriteString(" |\n")
	}

	b.WriteString("\n## How to read this\n\n")
	b.WriteString("- **Case accuracy** takes the majority answer per case; the interval is Wilson 95% over distinct cases, so small datasets give wide intervals.\n")
	b.WriteString("- **Brier** and **log loss** reward probabilities that are both right and honest; **ECE** is the average gap between stated confidence and observed accuracy (10 bins).\n")
	b.WriteString("- **AUROC** asks whether confidence separates right from wrong answers at all (0.5 = no, 1.0 = perfectly); it does not depend on any threshold.\n")
	b.WriteString("- **Selective accuracy** shows what a confidence threshold would buy: coverage is the share of answers the agent would act on.\n")
	b.WriteString("- **point** probabilities (no logprobs) put 1.0 on the answer, so their calibration metrics measure only accuracy.\n")
	_, err := io.WriteString(w, b.String())
	return err
}

func auroc(v float64) string {
	if v < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", v)
}

func ms(d time.Duration) string { return fmt.Sprintf("%dms", d.Milliseconds()) }

func sources(s Summary) string {
	var parts []string
	for src, n := range s.Sources {
		parts = append(parts, fmt.Sprintf("%s %d", src, n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
