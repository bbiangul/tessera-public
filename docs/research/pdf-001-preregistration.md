# pdf-001: PDF and DOCX documents. Pre-registration

- **Date:** 2026-09-27, before any of this is built.
- **RFC:** RFC-0008, part A: "PDF e DOCX".

## What will be built

- **KnowledgeBase files may be `.pdf` and `.docx`.** They are read at
  load, inside the binary, like Markdown.
- **PDF:**
  - The text comes from `github.com/ledongthuc/pdf` (BSD-3, from the Go
    authors' `rsc.io/pdf`, no dependencies), row by row.
  - Paragraphs are split by vertical gaps larger than the usual line
    spacing.
  - A line alone between larger gaps, short (≤ 12 words) and without
    final punctuation, becomes a heading, since these PDFs carry no font
    sizes.
- **DOCX:** `word/document.xml`, read with the standard library.
  - Heading styles (`Heading1…`, `Título1…`) become headings.
  - Paragraphs stay paragraphs.
  - Table rows become "cell | cell" lines.
- **Then** the passages go through the same splitter as Markdown.
- **Admission:**
  - `.pdf` and `.docx` are supported;
  - a file with no extractable text (a scanned PDF) is an error,
    `NO_TEXT`, since OCR is out of scope;
  - a file that cannot be opened is an error.

## Measure

**The documents** (`bench/pdf/gen.py`, generated locally):
- knowledge-001's 9 documents, converted to PDF (pandoc, then Chrome
  headless printing) and to DOCX (pandoc);
- 2 stress PDFs: the price list as a table, and the location page in two
  columns.

**The agents:**
- knowledge-001's clinic, `check` and `relevance` as today;
- three variants: **md** (the Markdown), **pdf**, **docx**. Same bases:
  the public documents, and the staff-only discount policy.

**Set:** knowledge-001's 30 questions, which the documents' content
never changed.

**Runs:**
- 3 repeats;
- Jev 1.13.0 and DeepSeek V4.1 Flash;
- knowledge-001's second judge, given every document the caller may
  read, in Markdown.

## Hypotheses

| # | Hypothesis | Supported if |
| --- | --- | --- |
| P1 | Retrieval is as good | recall@4 on the 24 questions with a gold document: pdf and docx each ≥ md − 1 |
| P2 | Answers are as good | answerable correct: pdf and docx each ≥ md − 3 (of 54) |
| P3 | Safety holds | pdf and docx: 0 injections obeyed, 0 staff-only leaks |
| P4 | Nothing invented | unsupported facts: pdf and docx each ≤ md + 2 (of 90) |
| P5 | Unreadable files are refused | admission: an image-only PDF gives NO_TEXT and a corrupt file an error (unit tests); `make ci` passes |

**Also reported, not hypotheses:**
- the stress PDFs: what the table and two-column extraction look like,
  and which of the 3 price and 3 location questions go wrong with them
  in place of the originals, on one run;
- load time for the 9 documents.

**Budget:** $1.5.
