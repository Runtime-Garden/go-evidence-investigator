# Evidence Investigator — JEV decision architecture

**Design & prototype implementation, v0.2 (2026-10-12).** This document describes the implemented optional `use_jev=true` path. It is **not** a claim of live TypeSafe API validation or a calibrated scientific classifier.

## Why JEV

Jev is TypeSafe AI's System One decision model. It accepts a `state` and named, typed `questions`, and returns choices, ordered scores and yes/no probabilities instead of explanations. Official docs: https://docs.typesafe.ai/introduction and https://docs.typesafe.ai/api . The canonical endpoint is `POST https://api.typesafe.ai/v1/systemone`.

## Implemented flow

```text
Claim
  -> BM25 retrieval (no LLM) -> source ID / SHA-256 exact-text checks
  -> Analyst LLM: one validated finding per retrieved passage (NO verdict)
  -> JEV: verdict Choice + needs_review Noul + evidence_quality Score
  -> Go policy: low-confidence and high-review gates; human review always required
  -> Narrator LLM: exactly the Go-policy verdict + cited readable summary
  -> Go validation: reject altered verdict or fabricated quotes
  -> JSON report + web UI
```

**JEV is the model that proposes the typed verdict.** The Go application alone decides whether the proposal can be displayed as-is or must be marked `REQUIRES_REVIEW`. The final LLM cannot revise either judgment; it can only explain it. All verdicts remain provisional, and the report always flags human review until the workflow is validated for the target dataset and risk.

## Analyzer contract

The upstream LLM returns exactly this JSON object, with one finding per retrieved passage:

```json
{
  "findings": [
    {
      "source_id": "synthetic-demo:1001:0",
      "quote": "lowered blood pressure",
      "relation": "SUPPORTS",
      "reason": "The synthetic source describes reduced blood pressure."
    }
  ]
}
```

The code rejects unknown source IDs, missing/duplicate passages, invented quotation text, invalid relations and empty or overlong reasons. Allowed relations are `SUPPORTS`, `CONTRADICTS`, `IRRELEVANT`, `UNCLEAR`. **These labels are the analyst LLM's interpretation, not independently proven semantic entailment**; evaluation is still necessary. No SciFact gold labels are used for inference.

## JEV request contract

The application sends one bounded state object:

```json
{
  "claim": "Synthetic treatment lowers blood pressure",
  "sources": [
    {"source_id":"synthetic-demo:1001:0", "title":"Synthetic study", "quote":"The treatment lowered blood pressure."}
  ],
  "findings": [
    {"source_id":"synthetic-demo:1001:0", "quote":"lowered blood pressure", "relation":"SUPPORTS", "reason":"Synthetic source supports the direction of the claim."}
  ],
  "policy": "provisional evidence evaluation, no autonomous scientific decisions"
}
```

Wrapped into the official `POST /v1/systemone` request:

```json
{
  "model": "jev-latest",
  "state": {"claim":"...","sources":[],"findings":[],"policy":"..."},
  "questions": {
    "verdict": {
      "type": "choice",
      "instructions": "Given the claim and findings, which provisional classification applies?",
      "criteria": {
        "SUPPORTED": "Direct evidence supports the claim without material contradiction",
        "REFUTED": "Direct evidence contradicts the claim without material supporting conflict",
        "INSUFFICIENT_EVIDENCE": "Available sources do not establish the claim",
        "REQUIRES_REVIEW": "Conflicting or difficult judgment"
      }
    },
    "needs_review": {
      "type": "noul",
      "instructions": "Does the evidence require qualified human review due to ambiguity or conflicts?"
    },
    "evidence_quality": {
      "type": "score",
      "instructions": "How direct and complete is the evidence?",
      "criteria": ["None", "Indirect", "Partially direct", "Direct and reasonably complete"]
    }
  }
}
```

The implementation has more detailed wording; the payload above is an illustrative schema, **not a captured API request or response**. The JEV questions are evaluated independently against the same state. Do not assume `needs_review` has observed another question's result.

## Go policy

- Validates model ID, answer types, expected labels, probability bounds, probability sum, and required `noul` and `score` values.
- Preserves JEV's `jev_choice`, probability distribution, confidence, model and token usage in the report.
- Prototype gates: Choice `confidence < 0.80` OR `needs_review.noul >= 0.65` -> exposed `decision.verdict=REQUIRES_REVIEW`. If neither gate applies, exposes JEV's typed `choice` unchanged.
- Human review remains **required for every decision**, including `SUPPORTED` and `REFUTED`. Gates are intentionally conservative, *illustrative*, and **not calibrated**. JEV confidence is not accuracy or proof.
- No retrieved evidence -> report `NOT_ASSESSED` without calling JEV. Any rejected LLM/JEV API response -> HTTP `502` without returning an invented verdict.

## Narrator contract

```json
{
  "verdict": "SUPPORTED",
  "summary": "The available synthetic evidence is consistent with the claim, subject to human review.",
  "citations": [
    {"source_id":"synthetic-demo:1001:0", "quote":"lowered blood pressure"}
  ]
}
```

The Go validator rejects a `verdict` different from the post-gate decision, unrecognized citations, nonverbatim quotes, overly long/empty summaries. It **cannot** guarantee the summary's semantic faithfulness or immunity to adversarial passages; those need dedicated evaluations and human review. Avoid using any JEV or LLM output to drive clinical, legal, security, financial, or other high-stakes actions autonomously.

## Configuration and API

```sh
export LLM_MODEL='YOUR_OPENAI_COMPATIBLE_MODEL'
export LLM_BASE_URL='https://your-model-provider.example/v1'
export LLM_API_KEY='...'
export JEV_API_KEY='...'
export JEV_MODEL='jev-latest'
# Optional; the default is the official TypeSafe endpoint
export JEV_ENDPOINT='https://api.typesafe.ai/v1/systemone'

go run ./cmd/evidence-investigator -mode=serve -data=data/demo
```

```sh
curl -sS -X POST http://127.0.0.1:8080/api/investigate \
  -H 'Content-Type: application/json' \
  -d '{"claim":"Low temperatures reduced tomato fruit yield in a synthetic greenhouse","top_k":5,"use_jev":true}'
```

Successful response adds `jev_report` with `analysis`, `decision`, `narrative` objects, plus the unchanged retrieved `evidence` array. `use_jev` and legacy `use_model` cannot both be true. The browser selects JEV by default **if configured**. Otherwise retrieval-only mode remains functional.

For reproducible benchmark experiments, prefer pinning `JEV_MODEL=jev-1.13.0` rather than the rolling `jev-latest` alias. The official model list currently identifies 1.13.0 as the stable release: https://docs.typesafe.ai/models . Always retain the model ID returned in the decision record.

## Production and evaluation gaps

- Live TypeSafe API and commercial model calls: **not executed**; localhost mocks test schema, ordering, rejection and fail-closed behavior.
- SciFact corpus download, verdict accuracy, evidence attribution accuracy, citation precision, calibration, thresholds and latency: **not measured**.
- Google ADK Go multi-agent workflow: **not integrated**. Sequential Go orchestration exists. ADK can later wrap these typed stages without changing contracts.
- Retry/backoff, durable runs, authorization, persistent audit logs, operational tracing, human approval UI and dataset bias tests: **pending**.
- The LLM provider receives the selected claim and source passages, and the JEV provider receives the claim, sources and model-created findings. Use public datasets or explicit disclosure permissions only.
- This package is not safe to expose as an unrestricted public inference endpoint: protect with trusted user auth / Cloudflare Access plus origin protection, quotas, TLS and logging policy. No incoming host port is configured in the Compose example.

## Evaluation plan

1. Use fixed SciFact dev split and a gold-label-to-four-way mapping, avoiding train/dev leakage.
2. Measure evidence document recall, sentence rationale precision/recall, analyst relation agreement, final verdict accuracy and macro-F1, abstention coverage and error rate for high-confidence subsets.
3. Compare baselines: retrieval only, direct LLM, LLM analyst + JEV, and LLM analyst + JEV + narrator.
4. Include counterfactual labels, conflicting reports, prompt injection in retrieved abstracts, missing citations, model outages and stale corpora.
5. Tune gate thresholds on a separate calibration subset, then report held-out metrics, per-run cost, p50/p95 latency and model revisions.

## References

- https://docs.typesafe.ai/introduction
- https://docs.typesafe.ai/api
- https://docs.typesafe.ai/cookbooks/citation_check
- https://docs.typesafe.ai/cookbooks/classifying_rag_passages
