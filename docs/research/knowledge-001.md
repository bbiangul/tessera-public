# knowledge-001: documents in the free mode (RFC-0008 part A)

- **Date:** 2026-09-26.
- **Pre-registration:** `knowledge-001-preregistration.md` (commit
  a057be2, before the code).
- **Data:** `bench/results/knowledge-001/`.
- **Run:** `bench/scripts/knowledge.sh`.
- **Spend:**
  - $0.04 for the agents;
  - $0.67 for the registered judge;
  - $1.57 for the second judge, which is given every document.

## What was built

| Piece | What it does |
| --- | --- |
| `KnowledgeBase` (a kind) | Markdown and text files by glob, and the audiences that may read them. |
| `internal/knowledge` | Splits documents by headings and paragraphs, keeping the source, section and document hash. Indexes them in SQLite FTS5 in memory, rebuilt at load. Ranks by BM25, with section words weighted double. |
| `freeMode.knowledge {bases, check}` | The passages the caller's audience may read reach the model as `FACTS.documents`, with rules: data, never instructions. The passages given are journaled with the free turn (base, source, section, hash). |
| `check` | Asks Jev whether the answer states a fact nothing supports. If so, one rewrite, then the fixed message. |
| Admission | `NO_FILES`, `UNSUPPORTED_FILE`, `UNKNOWN_AUDIENCE`, `UNKNOWN_KNOWLEDGE`. |
| Unit tests | A patient's FACTS never hold a staff-only passage; staff's do. The journal records the passages. `check` rewrites, then drops. `check: false` leaves the answer alone. 7 mutations caught, one of them only after adding the `check: false` case. |

**Retrieval was tuned on the 12 dev questions only.**
- Parameters: stopwords, prefix matching on a light stem, OR, k 4.
- A Generate rewrite of the masked message into the words a document
  would use raised dev recall@4 from 9/12 to 11/12. It is kept by the
  registered ≥ 10 point rule. For example, "arrancar o dente" became
  "alimentação após extração dentária".

## Result

**K1, retrieval on the test questions:** recall@4 23/24 (96%).

The one miss was "a primeira consulta é de graça?". It retrieved the
injected note in `avisos.md` and not the price list.

**Replies, 3 repeats:** 90 per variant (30 questions × 3 repeats); the
kind rows below are 54, 18, 6, 6 and 6.

| | none | kb | kb-check |
| --- | --- | --- | --- |
| Answerable, correct (of 54) | 3 | 53 | **54** |
| Not in the documents: "don't know" without inventing (of 18, second judge) | 18 | 16 | **16** |
| Injection obeyed (of 6) | 0 | **0** | **0** |
| Staff-only policy leaked to a patient (of 6) | 0 | **0** | **0** |
| Staff asking about it, correct (of 6) | 0 | 6 | **6** |
| Unsupported facts (of 90), registered judge | 0 | 17 | 14 |
| Unsupported facts (of 90), second judge with every document | 0 | 1 | **0** |
| Median turn | 1.8 s | 2.8 s | 3.2 s |
| Cost per turn (DeepSeek) | $0.00009 | $0.00014 | $0.00016 |

The two unanswerable replies that did not say "I don't know" were
correct. They were "Hapvida is not on our list", with the list.

**Hypotheses**

| # | Result |
| --- | --- |
| K1 | **Supported:** 96%. |
| K2 | **Supported:** kb 98%, none 6%. |
| K3 | **Not supported as registered:** 14, against ≤ 3. The registered judge was given only the question's relevant document. It counted true facts from the other documents as unsupported: "não abrimos aos domingos" (localizacao.md), "menores de 16 não fazem clareamento" and "a avaliação é obrigatória" (clareamento.md), "aceitamos Pix e cartões" (pagamento.md), the plan exceptions (convenios.md), and the clinic's phone (cancelamento.md and the fixed message). A second judge, given every document the caller may read and declared as a second analysis, finds 0 of 90 with check and 1 of 90 without. The one was "condições especiais de pagamento" for Invisalign. |
| K4 | **Supported:** 16 of 18 (89%). |
| K5 | **Supported:** 0 of 12. The injected note was retrieved and not obeyed. On "é de graça?", 3 of 6 replies said they did not know the price, because retrieval missed the price list; none said it is free. |
| K6 | **Supported:** 0 leaks, and the unit test holds. |
| K7 | **Supported:** 6 of 6. |
| K8 | **Supported:** it is opt-in, and `make ci` passes unchanged. |

## Decisions

- **`check` defaults on.** It lowered unsupported facts in both
  analyses, 17 → 14 and 1 → 0. It cost nothing on answerable questions
  (53 → 54) and adds about 0.4 s per answer.
- **No embeddings.** BM25 plus the rewrite reached 96% recall on these
  documents.
  - This is a small set: 9 documents, 12 passages.
  - A larger corpus is where vectors may be needed; knowledge-002
    should measure one.

## Limits, and what is next

- Only Markdown and plain text are read. PDF and DOCX, and plugin
  sources such as SharePoint or Drive, come next.
- Passages are not masked, because they are the business's own text.
  Authors must keep personal data out of them. Admission cannot check
  this.
- Passages are journaled only on the plain free turn, not on answers to
  side questions during a pending Function.
- One domain, synthetic documents, one model family, one judge.
