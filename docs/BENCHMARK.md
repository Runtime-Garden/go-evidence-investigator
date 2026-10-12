# Benchmark protocol — SciFact dev (planned external run)

## Dataset

Use [SciFact](https://github.com/allenai/scifact), specifically `corpus.jsonl` and labeled `claims_dev.jsonl`. This benchmark's results are **not** the official SciFact leaderboard. Training/test split must remain isolated and the unpublished gold test labels must not be guessed. The bundled `data/demo` is synthetic and cannot support scientific performance claims.

Download using the native Go CLI:

```sh
go run ./cmd/evidence-investigator -mode=fetch -data=data/scifact
```

Then compute baseline document-hit retrieval recall:

```sh
go run ./cmd/evidence-investigator -mode=eval -data=data/scifact -k=5
```

Metric here is whether **at least one** gold evidence document occurs among the top-k *sentences*. This is **not** gold sentence rationale recall. Retrieval is lexical BM25, without gold labels as features.

## Three-arm comparative protocol

Configure LLM and JEV keys, pin models for a reproducible run and set a small budget-capped sample count:

```sh
go run ./cmd/evidence-investigator -mode=benchmark \
  -data=data/scifact -k=5 -limit=10 \
  -benchmark-output=benchmark-details.jsonl > benchmark-summary.json
```

The benchmark uses exactly the same claim and retrieval results for each arm:

| Arm | Calls | Reported verdict |
|---|---|---|
| `llm_only` | Direct LLM evidence assessment | LLM tentative verdict |
| `llm_plus_jev` | LLM structured findings + Jev + Go policy | Jev/policy verdict |
| `llm_jev_narrator` | Same shared Jev result + extra LLM narration | **Same** Jev/policy verdict, never an independent prediction |

The last arm measures presentation cost/latency, **not** a third independent verdict model. JSONL details retain dataset claim identifiers, labels, predictions and errors but no API keys.

Metrics: per-arm attempted, correct, abstained, failed, accuracy-on-attempted, coverage and summed latency; JEV multiclass Brier and 10-bin ECE on successful JEV answers, LLM usage tokens and JEV usage tokens. These metrics are **exploratory estimates** on the dev set with a maximum of 50 eligible samples per invocation, so report uncertainty/sample size. `REQUIRES_REVIEW` and `INSUFFICIENT_EVIDENCE` count as abstentions and do not count as correct for labeled SUPPORT/CONTRADICT cases.

## Limitations and reproducibility

- Gold verdict is only included when evidence annotations agree on a single SUPPORT or CONTRADICT label; mixed/unlabeled cases are skipped. This changes the population and must be reported.
- The current benchmark measures **a small online sample**, not representative scientific performance. `-limit` selects the first N labeled examples; randomize deterministically in a future iteration for stronger generalization.
- Provider versions, prompt revisions, token pricing, time and network region affect costs and latencies. We report usage tokens, **not fabricated EUR/USD spend**. Log actual invoices for cost comparisons.
- LLM/JEV may be non-deterministic. Repeat trials and compute bootstrap confidence intervals before any quality claim. Lower confidence is not equivalent to improved calibration.
- Domain experts must spot-check groundedness, explanation accuracy and ambiguous/incomplete evidence. Verbatim quote checks alone cannot detect subtle semantic distortions.
- Current execution does not download fixtures automatically from CI, nor call paid services from CI. Live eval requires explicit credentials and quota approval.
