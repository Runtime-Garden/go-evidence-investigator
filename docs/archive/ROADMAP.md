# Evidence Investigator — Product Requirements and Technical Roadmap

**Project code:** `evidence-investigator`  
**Product role:** experimental, explainable evidence research assistant  
**Classification:** independent Runtime Garden Lab / candidate open-source Project  
**Status:** v0.1 core implemented and tested with synthetic fixture; dataset benchmark, model quality and deployment unverified.

## Problem statement

An LLM can generate a plausible answer while failing to provide verifiable support. Evidence Investigator aims to separate source retrieval from claim interpretation, preserve sentence-level provenance, refuse unsupported certainty, and provide a human-review gate.

## Functional requirements

| Priority | Requirement | Status in v0.1 |
|---|---|---|
| P0 | Load public JSONL corpus and sample claims | Implemented; SciFact schema supported |
| P0 | Retrieve ranked exact evidence sentences | Implemented, lexical BM25 |
| P0 | Return document title / ID / sentence index / exact quote | Implemented |
| P0 | Distinguish retrieval score from claim truth | Implemented; NOT_ASSESSED by default |
| P0 | Do not leak benchmark gold evidence into inference | Implemented by API and LLM request design |
| P0 | Reproduce and benchmark evidence retrieval | Implemented eval CLI; real run pending |
| P0 | Validate model quote membership | Implemented for optional provider; model quality pending |
| P0 | Always mark model outputs as needing human review | Implemented in JSON and UI |
| P1 | Pluggable Google ADK Go 2.x multi-agent workflow | Not implemented |
| P1 | Persist explicit human accept/reject and analyst notes | Not implemented |
| P1 | Hybrid BM25+vector retrieval and reranking | Not implemented |
| P1 | Research-style experiment report and controlled evaluation | Not implemented |
| P1 | Authentication, authorization, rate limits and audit logs | Not implemented |
| P2 | Multi-document PDF ingestion and revision handling | Not implemented |
| P2 | Web search connectors, provenance lineage, temporal conflicts | Not implemented |
| P2 | Interactive claim decomposition and iterative research budget | Not implemented |

## Suggested Google ADK Go v2 topology (planned, NOT implemented)

```
Input claim
    │
    ▼
Query Planner (LLM)
    │
    ▼
Evidence Retriever (read-only Go BM25 / future hybrid index)
    │
    ▼
Evidence Consistency / Conflict Investigator (limited model calls)
    │
    ▼
Citation Verifier (deterministic source ID + quote + contextual check)
    │
    ▼
Draft Assessment (abstain when confidence/coverage low)
    │
    ▼
Analyst Review → Signed report / benchmark log
```

Integrate the existing Go retrieval engine as a *read-only tool*. Google ADK Go v2 officially requires Go 1.25+ and uses module `google.golang.org/adk/v2`; verify actual methods and pin a tested release in a separate commit rather than inventing a framework integration. Prefer a simple sequential graph for MVP; do not split into multiple deployed microservices. DeepSeek Harness is separate, not a required dependency.

## Core data contracts

- `Claim`: id, text, optional corpus/split metadata.
- `Evidence`: source ID, doc ID, sentence index, title, text, checksum, lexical retrieval score.
- `Proposal`: verdict, explanation, exact source quotations, model identification, review-required flag.
- **Future `Review`**: analyst ID, proposal/run ID, acceptance or rejection, reason, timestamp, dataset version, model/prompt version, final outcome.

## Evaluation plan

Split discipline: use SciFact **train** only for development, **dev** for repeatable benchmarking; do not report test accuracy without test labels. Record dataset archive SHA, Git commit, retrieval parameters, Go version, model name/version, seed, cost and latency. Never evaluate against a corpus reduced to gold-only documents (that would leak evidence identity).

Recommended measurements:

1. **Document recall@k** on SciFact dev claims with nonempty annotated evidence — implemented.
2. **Sentence rationale recall@k** and claim-level evidence precision — future.
3. **Verdict macro F1 and abstention coverage**, using independently selected test claims — future.
4. **Citation exactness** and reviewer-verified *semantic faithfulness* — current exact-substring gate, human metric future.
5. **Conflicting-evidence handling** and unsupported-claim rate — future.
6. **Latency p50/p95, token usage and cost/run**, including network/model failure handling — future.
7. **Access and prompt-injection tests** including unrelated doc instructions, cross-document leak, poisoned passages — future.

No performance percentages should be advertised based only on the tiny synthetic fixture. A meaningful success condition for P1 is explicit thresholds agreed *after* running and reporting the true baseline.

## Phased delivery

**Phase 0: Data and reproducible baseline (v0.1 — locally tested).** Repository, exact corpus ingestion, retrieval, source IDs, safe browser UI, offline eval CLI, Docker and tests. Remaining: real SciFact download, repeatable benchmark and runtime resource measurements.

**Phase 1: Research-grade evidence analysis.** Add neural retrieval/reranker behind feature flags, semantic citation verifier, controlled LLM provider with retries/timeouts/budgets, evaluation harness and a persistent case file. Test real SciFact dev results and failure cases.

**Phase 2: Google ADK Go orchestration.** Build typed tool wrappers, bounded graph workflow, model proposals, contradiction checks, immutable audit trail and explicit human sign-off. Add RBAC before multi-user operations.

**Phase 3: Public demonstration.** Security audit, scoped authentication, upload governance, dataset attribution UX, rate limiting, resource quotas, deployment validation, versioned documentation and a reproducible Runtime Garden Lab report.

## Acceptance checklist before labeling v1.0

- Real SciFact evaluation with saved run metadata and independent relevance checks.
- All nontrivial claims link to exact passage IDs and source document metadata; no invented URLs.
- Empty/ambiguous evidence leads to abstention or review, not authoritative assertions.
- Prompt injection, malformed JSON, provider outage and timeouts handled without exposing secrets.
- Explicit analyst approval recorded, access policies tested, no writes permitted through retrieval tools.
- Container reproducibility, hardened runtime, backup/restore where applicable, rollout/rollback and actual VPS telemetry verified.
- Public project documentation, dataset rights and disclosure of limitations completed.


## v0.2 addition — JEV decision model (2026-10-12)

- [x] Structured cited analyst findings, independently of the final verdict.
- [x] TypeSafe Jev typed Choice/Score/Noul adapter; API mocked and validated.
- [x] Application-controlled low-confidence/review fallback.
- [x] Constrained LLM report writer that cannot override the decision field.
- [x] Backward compatible optional API/UI mode and mock unit tests.
- [ ] Official TypeSafe API + LLM integration smoke tests with actual credentials.
- [ ] SciFact quality and confidence calibration benchmarks.
- [ ] Production auth, audit events, retries and observability.
- [ ] ADK Go orchestration and human-review decision sign-off.
