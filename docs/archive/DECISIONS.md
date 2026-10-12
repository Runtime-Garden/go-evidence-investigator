# Architectural decision record (initial)

## ADR-001 — Bounded public corpus first

**Decision:** Use SciFact before web crawling. **Why:** publicly described JSONL schema and source-level annotated evidence make retrieval objectively testable. **Trade-off:** domain is scientific and incomplete for general claims; no time-varying web verification. **Revisit:** after genuine SciFact dev results and recorded errors.

## ADR-002 — Go standard library BM25 baseline

**Decision:** Use a self-contained Go index and HTTP server before adopting large vector/database dependencies. **Why:** simple, transparent, cheap, minimal RAM and operational footprint at SciFact scale. **Trade-off:** paraphrase mismatch and poor semantic recall relative to capable neural retrievers. **Revisit:** benchmark Recall@5 and rationale recall on the real dev split.

## ADR-003 — Model only proposes; verifier checks source quote

**Decision:** Default to `NOT_ASSESSED`; optional provider classification always requires human review. Reject source IDs not retrieved and quotes not contained verbatim in the source sentence. **Why:** lexical similarity is not entailment and LLM outputs are untrusted. **Trade-off:** increased abstention, human work and possible false confidence if the user mistakes quote membership for validity. **Revisit:** after a verified semantic entailment evaluation suite.

## ADR-004 — Runtime Garden-compatible independent stack

**Decision:** one hardened Go container, no database or additional runtime, private data mount, external `proxy` only when publishing via NPM. **Why:** lowest reasonable moving-part count; follows existing hosting architecture. **Trade-off:** no persistent multi-user case history. **Revisit:** when authentic user-review workflows require durable data.

## ADR-005 — ADK integration later

**Decision:** do not claim ADK implementation in v0.1. Preserve a framework-neutral read-only search boundary for ADK Go v2 typed tools later. **Why:** separates evidence quality from orchestration complexity; tests available even without model API keys. **Trade-off:** no multi-agent workflow currently. **Revisit:** Phase 2.
