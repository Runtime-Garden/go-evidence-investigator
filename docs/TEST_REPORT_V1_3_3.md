# v1.3.3-rc1 — Bounded convergence fallback

**Status:** Implemented and offline-tested (2026-10-12). **Not** real-provider or live-deployment verified.

## User-observed failure

The local WSL integration log showed `autonomous research stopped safely without final report (turns=9, decisions=0, evidence_revision=2)` after the previous semantic action validation hotfix. Provider configuration had succeeded, but no JEV decision was reached.

## Root cause in v1.3.2 code

When the inbox ran dry, the router retried sending a standard deduplicated `system -> analyst` message. If an analyst had already received that topic in the same evidence revision and finished without requesting `decide`, the deduplicator silently dropped the retry. The runtime returned without making a decision. The same issue could prevent a repeat narrator request.

## Change

- Only after peer-message exhaustion or explicit budget/convergence stop, the router creates one **bounded internal** Analyst or Narrator message without using the peer-message deduplicator.
- The internal Analyst action is `decide` (guarded LLM evidence analysis, TypeSafe JEV decision, deterministic policy gates); the internal Narrator action is `report` (validated final prose locked to the provisional decision). No AI agent may override these safeguards.
- Every fallback is recorded as a `bounded_finalization` event with target and evidence revision.
- Terminal stop errors now include LLM call count and stop reason.
- Existing task ACLs, JEV and citation verification, per-request budgets, human-review requirement, and read-only corpus tools remain unchanged.

## Verification

- Regression test `TestQueueExhaustionTriggersBoundedJEVFinalization`: **fails** against v1.3.2; **passes** against v1.3.3. Uses mock LLM and mock JEV.
- Existing `TestNarratorReopensResearchAndInvalidatesDecision`: **passes**; a challenge can still cause new evidence, a second JEV decision, and a report.
- Offline `GOPROXY=off go test ./...`: **passes**.
- Offline `GOPROXY=off go vet ./...`: **passes**.
- Offline `GOPROXY=off go test -race ./...`: **passes**.
- Offline `GOPROXY=off go build ./cmd/evidence-investigator`: **passes**.

## Explicit limitations

No live Mistral or OpenRouter JEV/Mercury Decide request was executed in this environment. The user must verify endpoint/schema compatibility, model quota, real structured outputs and UI report on WSL. Docker/VPS deployment and SciFact online benchmarks are not verified by this patch.
