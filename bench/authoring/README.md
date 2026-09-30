# External partner authoring

Build the CLI with `make build`, then build a fresh directory outside the
repository (macOS sandbox-exec is required for the prescribed author workflow):

```
python3 bench/authoring/build_kit.py /private/tmp/tessera-partner-new \
  --deny /private/tmp/tessera-partner-previous
```

Pass every previous or parallel kit with `--deny`. Start an author with no
conversation history and only its kit path/instructions. The shared host is
not a VM; the shell profile blocks source reads, but other tools are restricted
by the protocol, not a global permission boundary. Probe that boundary first.
The allowlist includes a compiled binary, generic examples, product guides,
a skill, a synthetic clinic contract/brief and a budgeted CLI launcher.
It excludes sources, repository instructions, clinic solutions and evaluation
sets. Missing Markdown guide links are explicitly converted to unavailable
background references and listed; inline source references may remain as
citations. This does not claim complete documentation dependency closure.

Credentials are injected by the launcher from the evaluator's configured
machine, never included in a kit. This launcher is a controlled experiment
audit/budget aid, not an adversarial security boundary. Budget is twenty live
tests with $.05 each; replay is free. Record a manifest and immutable copy of
the submitted definitions, reports, calls and evidence before evaluating.
Never return held-out failures to the author to improve a submitted score.

`--guidance` adds an experimental guide to the kit. Authoring-004 uses the
frozen `bench/results/authoring-004/candidate-guidance.md`. Its two initial
kits were assembled before this reusable builder; their exact supplied files
and hashes are preserved under that result directory. Always compare the
frozen files when reproducing a historical measurement.
