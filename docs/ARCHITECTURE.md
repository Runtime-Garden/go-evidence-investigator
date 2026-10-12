# Architecture and Contracts — v1.3.0-rc1

## Decisions

- **Native Go event-driven agent orchestration** replaces the earlier sequential research workflow and optional Google ADK dependency. Rationale: no AI provider SDK; there are no third-party Go dependencies; structured JSON validation is implemented locally for the response DTO subset. Migration to ADK Go v2 would require Go 1.25+ and the ADK library, without improving the basic safety constraints. This change is intentional and testable.
- The **LLM does not make the final decision**. An LLM analyst produces exactly one structured finding per source passage. All `source_id` and verbatim quotes must match retrieved corpus passages. The Jev model receives only claim, exact source texts, checked findings, and explicit bounded criteria.
- Jev returns `choice` verdict (4 labels), `noul` human-review probability, `score` evidence quality. Official endpoint: `POST https://api.typesafe.ai/v1/systemone` with model, state and questions. Responses are checked for type, range and completeness. Return to human review when confidence < 0.80, review probability >= 0.65, evidence score < 1.5, contradictory findings, or JEV verdict unsupported by labeled findings. **Thresholds are uncalibrated prototype choices**.
- The **LLM narrator is not a decision-maker**. A mismatched verdict or invented citation fails closed. Semantic distortions of a valid citation remain possible and must be checked by human reviewers.
- Every successful investigation is stored with unique 128-bit random ID, a pending review event, and JSON report. Later reviewer decisions append a hash-linked event; every store read rechecks integrity. SHA-256 is tamper-evident against accidental/uncoordinated edits, **not** a signature against an attacker with full write access to the volume.
- Async queue, vector DB and Kubernetes are intentionally absent. Read-only inference in a bounded, synchronous request with a short deadline is sufficient for MVP traffic. `use_agents` invokes bounded peer-to-peer messages, with no required fixed stages. See `AUTONOMOUS_AGENTS.md`.

## Stages

```text
HTTP auth -> validation -> BM25 retrieval -> passage digest check
   -> analyst HTTP completion (strict response_format JSON Schema) -> built-in strict DTO validation -> typed Go DTO validation
   -> Jev HTTP systemone -> typed answer validation
   -> policy / uncertainty gate -> narrator HTTP completion (same strict schema + validation)
   -> immutable verdict & citation checks -> atomic saved case
   -> authorized reviewer (separate token) -> review audit event
```

## Key JSON contracts

LLM analyst:

```json
{
 "findings":[{
   "source_id":"scifact:123:2",
   "quote":"exact substring from the original evidence",
   "relation":"SUPPORTS",
   "reason":"Why the quote bears on the claim"
 }]
}
```

A finding for every provided passage, no duplicates, no unknown relations. Allowed: `SUPPORTS`, `CONTRADICTS`, `UNCLEAR`, `IRRELEVANT`. Claims, source text and annotations are separate; no SciFact gold labels are sent to agents.

Jev request (illustrative):

```json
{
 "model":"jev-latest",
 "state":{"claim":"...","sources":[],"findings":[],"policy":"provisional evidence evaluation"},
 "questions":{
   "verdict":{"type":"choice","instructions":"...","criteria":{"SUPPORTED":"...","REFUTED":"...","INSUFFICIENT_EVIDENCE":"...","REQUIRES_REVIEW":"..."}},
   "needs_review":{"type":"noul","instructions":"..."},
   "evidence_quality":{"type":"score","instructions":"...","criteria":["No pertinent evidence","Mostly indirect","Partially direct","Direct evidence"]}
 }
}
```

Narrator:

```json
{"verdict":"REQUIRES_REVIEW","summary":"Plain-English, provisional, source-grounded explanation.","citations":[{"source_id":"scifact:123:2","quote":"exact substring"}]}
```

### HTTP client implementation

Only `net/http`/`encoding/json`, bearer credentials from environment, HTTPS required except explicit localhost testing. Built-in request timeouts and max response caps. Two short retries only for 429/502/503/504 or transport errors. Model outputs are JSON decoded with `DisallowUnknownFields` and rejected if extra text follows.

### Known limitations

- No external source browsing; only bounded, curated corpus is used.
- Retrieval is sentence-level BM25; multi-sentence rationales may be missed.
- Approver identity is bound to a single configured reviewer token, not individual SSO accounts. Cloudflare Access plus origin restrictions recommended; a different system is needed for multi-user production RBAC.
- Case status change is one-way. Review after approval requires a new case, not silent overwrites.
- High model confidence does not prove verdict correctness; validation only enforces a deterministic subset of contract and citation checks.

For Go DTO to wire-schema generation and rejection policy, see `docs/STRUCTURED_OUTPUT.md`.
