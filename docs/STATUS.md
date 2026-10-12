# Historical release evidence ledger (v1.1/v1.3; superseded by v1.3.1 below) — 12 October 2026

| Milestone | Implementation | Live/external verification |
|---|---|---|
| 1. Real JEV + LLM calls | Direct HTTP clients, strict JSON schemas, retry/timeouts | **Blocked:** credentials/provider accounts not shared |
| 2. SciFact benchmark | Native fetch CLI, retrieval evaluation, 3-arm online protocol | **Blocked:** real remote dataset + paid provider run not performed here |
| 3. Multi-stage orchestration | Native Go stages (analyst, JEV, narration); no mandatory ADK dependency | Native workflow mock-tested locally, **not ADK SDK integration** |
| 4. Human-in-the-loop | Reviewer API + separate token + versioned durable cases + web UI | Server review endpoint and file persistence locally tested; browser session and host persistence require deployment test |
| 5. Security & observability | Auth, bounds, no body logs, JSON metrics, integrity trail | Threat model documented; live origin/cost controls not audited |
| 6. Docker/VPS | Scratch image, Compose, network + volume, deploy scripts | **Blocked:** no Docker CLI or SSH credentials in this session |
| 7. Final verification & publish | Unit/integration suite, JS/shell syntax checks, local HTTP smoke + docs, packaging | Local build and tests passed; no GitHub publish, DNS or public demo claimed |

The source package can be used to reproduce local testing. No external service or public URL is marked deployed. A successful mock test does not prove a production API call. Before making public scientific claims, run the paid-provider benchmark and human review procedure described in `BENCHMARK.md`.

## v1.1: Typed structured output

**Implemented in source, not locally build-verified:** JSON Schema generation from Go DTOs, TypeSafe-independent post-response schema validation, explicit provider strict `response_format`, refusal/truncation rejection, typed JSON decoding, duplicate property rejection, and mock contract tests. The only new direct dependency is `jsonschema/v6 v6.0.3`; `golang.org/x/text` is transitive. Module-download DNS restrictions prevented revised Go tests and Docker build in this execution session. Do not mark this patch tested/deployed until completed on a networked environment.

## v1.3.0-rc1 peer messaging extension (2026-10-12)

- **Implemented in source:** event-driven agent inbox, typed peer messages, local read-only tools, backward delegation from narrator to researcher, evidence revisions, JEV decision invalidation, guarded final narration, hard turn/tool/model/time budgets, and agent event trace.
- **Tests authored:** permission tests and mock conversation with narrator -> researcher -> new evidence -> repeat JEV.
- **Not executed here:** Go tests due missing downloadable `jsonschema/v6` module (DNS denied). Provider integration, Docker image, browser test, SciFact paid evaluation and VPS deployment not verified.

## v1.3.1-rc1 — dependency-free structured outputs (12 October 2026)

- Removed `jsonschema/v6` and transitive Go modules; `go.mod` now has zero third-party requirements and there is no `go.sum`.
- Added a purpose-built local validator for the exact Go DTO schema subset used by this project. It is not a full JSON Schema standard implementation; no third-party validator code was copied.
- **Verified offline:** `go test ./...`, `go test -race ./...`, `go vet ./...`, executable build, and synthetic 5/5 retrieval smoke evaluation.
- **Still pending:** real TypeSafe JEV and LLM API access, SciFact benchmark, browser review session, Docker build, VPS deployment and published demo.
- Historical notes above document older snapshots and are superseded by this section for the current version.

## v1.3.2-rc1 — invalid peer search recovery (12 October 2026)

Reproduced semantic action-validation failure from user log (`critic: invalid search request`). Added one bounded corrective turn with an audit event; repeated failures are skipped and redirected to a guarded Analyst if appropriate. Full offline Go tests, race tests, vet, build and synthetic retrieval evaluation passed. Live provider E2E and VPS deployment still pending. See `TEST_REPORT_V1_3_2.md`.

## v1.3.3-rc1 — bounded convergence fallback (12 October 2026)

Fixed missing JEV decision after agent inbox exhaustion: a previously deduplicated analyst/narrator retry could silently fail. A strictly router-owned, budget-checked finalization message now forces the guarded Analyst -> JEV -> Narrator path when possible, with an explicit audit event; no synthetic verdict is generated. Reproduced failure on v1.3.2 and passed regression and all offline Go/vet/race/build tests on v1.3.3. Real provider and production E2E remain unverified. See `TEST_REPORT_V1_3_3.md`.
