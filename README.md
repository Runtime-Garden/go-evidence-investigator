# Evidence Investigator

> Runtime Garden · evidence-first AI research experiment · **v1.3.2-rc1 (bounded peer-agent action correction; local tests passed, live providers not fully verified)**
>
> Where ideas grow into systems.

A compact, auditable **scientific-claim investigation** prototype. It retrieves passages from a bounded corpus, validates verbatim citations, asks an LLM to label passage/claim relationships, asks **TypeSafe Jev** to make a typed provisional decision, applies deterministic safeguards, and asks an LLM to render that decision as a readable report. An authorized human records the final review disposition.

**No AI SDK, JSON Schema library, or other third-party Go module is used.** The module now has **zero external Go dependencies**. Provider calls use `net/http` and JSON; strict response contracts are generated and checked using `encoding/json`, `reflect` and `math/big`. Google ADK was considered, but its SDK/runtime dependency is intentionally not bundled; a native event-driven, peer-to-peer Go agent runtime is used instead. This is a deliberate trade-off, not a claim that ADK is installed.

## Status and boundaries

| Feature | Status |
|---|---|
| BM25 with sentence provenance and SHA-256 | Implemented; synthetic tests available |
| Schema-constrained LLM analysis -> Jev -> schema-constrained narration | Implemented; network calls need real credentials |
| Provisional decision consistency policy | Implemented; thresholds not calibrated |
| Persistent reviewer ledger, optimistic lock, chain hashes | Implemented; local tests |
| API bearer auth + separate reviewer credential | Implemented; single-token small lab scope |
| JSON stage metrics, bounded concurrency, retry/timeout | Implemented |
| SciFact fetch, retrieval eval, three-arm benchmark | Implemented; external benchmarking still requires dataset + credentials |
| Docker/NPM/Dockge stack | Files provided, not deployed from this workspace |
| Public live demo / external end-to-end provider tests | **Not verified** |

The answer is **not** a medical, legal or scientific finding. BM25 score is lexical relevance, not entailment. Jev confidence is not automatically empirical accuracy. `SUPPORTED` and `REFUTED` are *provisional evidence interpretations*. Final status is always pending human review until explicit authorization is recorded.

## Quick start (Go 1.23+)

```bash
go version
# No Go dependencies to download; can test offline
GOPROXY=off go test ./...
# local demo only, listens on 127.0.0.1:8080, no API auth by default
go run ./cmd/evidence-investigator -mode=serve -data=data/demo
# Open http://127.0.0.1:8080
```

For any **non-loopback** listener the server refuses to start without two distinct 24+ character tokens and `APP_REVIEWER_ID`. Create `.env` from `.env.example`; to use `.env` with `go run`, export the variables in your shell or load them through a protected shell environment. The Go app does **not** parse `.env` files itself. Do not commit secrets.

```bash
# Generate two DIFFERENT tokens
openssl rand -hex 32
openssl rand -hex 32

# Optional: download real public SciFact (no Python needed)
go run ./cmd/evidence-investigator -mode=fetch -data=data/scifact
# Retrieval-only benchmark; gold labels NEVER included in inference prompts
go run ./cmd/evidence-investigator -mode=eval -data=data/scifact -k=5

# With BOTH LLM_* and JEV_* credentials set:
go run ./cmd/evidence-investigator -mode=benchmark \
  -data=data/scifact -limit=10 -k=5 -benchmark-output=benchmark-details.jsonl
```

Provider env:

```
LLM_BASE_URL=https://api.openai.com/v1
LLM_MODEL=your-chat-completions-compatible-model
LLM_RESPONSE_FORMAT=json_schema
LLM_API_KEY=...
JEV_ENDPOINT=https://api.typesafe.ai/v1/systemone
JEV_MODEL=jev-latest
JEV_API_KEY=...
```

`LLM_BASE_URL` must be the API **base URL** (including `/v1`), not the `/chat/completions` endpoint. A compatible response must expose `choices[0].message.content` containing JSON. `LLM_RESPONSE_FORMAT=json_schema` (default) sends `response_format` with a Go-type-derived strict JSON schema directly over HTTP. If your model/provider explicitly does not support `json_schema`, opt into `LLM_RESPONSE_FORMAT=json_object`; local JSON Schema validation remains mandatory, but the provider then cannot guarantee schema-constrained generation. Unsupported response formats **do not automatically fall back**. For refusals, truncated responses, missing fields, invalid enums, unknown fields, duplicate keys, or non-JSON the pipeline rejects the result rather than issuing a verdict. No LLM model can guarantee semantic truth solely from schema enforcement. Because no vendor SDK is used, not every third-party OpenAI-like implementation is guaranteed compatible. Jev uses the official TypeSafe `state` + `questions` contract; verify model availability for your account via `GET /v1/models`.

## API

An `Authorization: Bearer <APP_AUTH_TOKEN>` header is required when configured. Never put credentials in the URL or expose it in frontend source. The browser workbench accepts the token into **in-memory JavaScript state only**, and an additional reviewer secret for final review.

| Route | Function |
|---|---|
| `GET /healthz` | Liveness only, not a downstream provider check |
| `GET /api/info` | Dataset, available providers |
| `GET /api/claims?limit=10` | Example claims (no gold labels) |
| `POST /api/investigate` | Retrieval / optional JEV pipeline and case creation |
| `GET /api/cases?limit=30` | Case ledger (authenticated) |
| `GET /api/cases/{id}` | Case and immutable review event history |
| `POST /api/cases/{id}/review` | Explicit reviewer authorization and optimistic concurrency |
| `GET /api/metrics` | In-process stage counts, errors and elapsed time |

```bash
curl -fsS -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $APP_AUTH_TOKEN" \
  -d '{"claim":"Green leaves absorb light","top_k":5,"use_jev":false}' \
  http://127.0.0.1:8080/api/investigate
```

Review payload (must additionally send `X-Review-Token: <APP_REVIEW_TOKEN>`):

```json
{"version":1,"status":"approved","actor":"authenticated-reviewer","note":"Validated the cited passages against original sources."}
```

Other review statuses: `rejected`, `needs_more_evidence`. The server uses the configured `APP_REVIEWER_ID` as the authoritative reviewer identity when set. Stale versions are rejected with `409`. A review is not changeable in v1; reopen as a new case. Case records are JSON files stored with mode `0600` in `CASE_DIR`.

## Autonomous multi-agent investigation (v1.3)

Set `"use_agents": true` on `POST /api/investigate`. **Planner, Researcher, Critic, Analyst and Narrator** communicate through typed message envelopes. Any role can delegate to another role, including backwards in the investigation. A narrator may send a new research task to an earlier agent. Every new source changes the evidence revision and invalidates previous JEV decisions. New analysis + JEV decision are mandatory before that revision can be narrated.

All routing and tool access permissions are enforced in Go; the agent's JSON is a *proposal*, not permission. The search/inspect/expand tools are restricted to local public corpora. No external browsing or mutable actions. Execution is time-, message-, turn-, tool- and API-budget limited; there is no uncontrolled forever-loop. **Peer messages are event-driven, not a fixed pipeline, though each event is dispatched serially.** Read [Autonomous Agent Architecture](docs/AUTONOMOUS_AGENTS.md) for examples and guardrails.

Example request: `{"claim":"Tomato plants exposed to lower greenhouse temperatures produced fewer fruits.","top_k":5,"use_agents":true}`.

## Pipeline

```text
claim -> BM25 -> passage SHA-256 audit
      -> LLM evidence analyst (strict JSON Schema response format + schema validation + typed Go DTO)
      -> strict source/quote validation
      -> TypeSafe Jev (choice + noul + score)
      -> Go decision policy gate
      -> LLM narrator (strict JSON Schema + immutable verdict + source checks)
      -> case ledger (pending_review)
      -> separate human reviewer decision
```

The static `use_jev` mode retains the original flow for reproducible ablation. The new `use_agents` mode uses the peer-to-peer runtime to revisit sources before reaching a JEV decision. The Go DTOs are the schema source of truth; string enums use `enum:"..."` struct tags. All LLM response types are validated by `internal/structured` (standard-library-only, intentionally limited JSON Schema subset) before any JEV request or public report. Deterministic source and decision checks remain separate from shape validation.

No hidden autonomous final judgments, internet search, file uploads, tenant-isolation claims, medical expertise, or browser source publication. Prompts treat source text as **untrusted data**; quote verification checks membership, **not semantic truth**. Review narrative for subtle unsupported paraphrases.

## Repository

```
cmd/evidence-investigator/      CLI + HTTP server
internal/investigator/         BM25, SciFact ingest, eval
internal/assessment/           direct OpenAI-compatible HTTP + JSON
internal/decisionflow/         analyst, Jev, policy, narrator
internal/multiagent/           typed messages, peer-to-peer dispatcher, source revisions
internal/httpjson/            bounded HTTPS request/retry
internal/casebook/            atomic review ledger + hash chain
internal/server/              REST + embedded light-first UI
internal/telemetry/           internal metrics
automation: .github/workflows/ci.yml
compose.yaml / Dockerfile      scratch + non-root stack
docs/                         threat model, runbook, benchmark
```

## Data and attribution

The **bundled demo** is synthetic. SciFact source archive is downloaded only on request. SciFact claims/annotations are CC BY 4.0; the abstract corpus is ODC-By 1.0; upstream code is Apache 2.0. See the [SciFact repository](https://github.com/allenai/scifact) and [license](https://github.com/allenai/scifact/blob/master/LICENSE.md). Do not automatically redistribute an entire downloaded dataset inside a source release.

## Related documentation

- [Architecture / contracts](docs/ARCHITECTURE.md)
- [Benchmark protocol](docs/BENCHMARK.md)
- [Security and limitations](docs/SECURITY.md)
- [Deployment, backup and rollback](docs/RUNBOOK.md)
- [Progress and external verification blockers](docs/STATUS.md)
- [Current offline test report](docs/TEST_REPORT_V1_3_1.md)

This project uses the Apache-2.0 license (see `LICENSE`).

## v1.3.1 output contract (replaces v1.1 dependency)

- **Single source:** Go response structs define required keys, nested types and string enums. `internal/structured` derives strict JSON Schema from them. All fields are required; unknown fields forbidden.
- **Provider guardrail:** Direct `POST /chat/completions` includes `response_format.type=json_schema`, `json_schema.strict=true` and the generated schema (default). No AI SDK.
- **Independent validation:** the built-in strict contract validator checks required properties, types, enums, arrays, nested objects and additional properties. Duplicate keys, refusals, incomplete responses and trailing content are rejected. A strict Go decoder then maps the object into the destination struct.
- **Semantic guardrail:** Analyst source IDs, verbatim citations, one-label-per-passage and immutable JEV/policy verdict checks remain in place. JSON Schema is not a fact-checker.
- **Failure policy:** Bad provider outputs cause an error; they are never silently coerced into valid case decisions. `json_object` is an **explicit** alternative for compatible models and still enforces local schema validation.

**v1.3.1 offline verification:** `GOPROXY=off go test ./...`, `GOPROXY=off go test -race ./...`, `GOPROXY=off go vet ./...`, `GOPROXY=off go build ./cmd/evidence-investigator` and the synthetic retrieval evaluation passed in the authoring environment. Provider credentials, live JEV and LLM calls, full SciFact benchmarks, Docker image build and VPS deployment remain unverified. This is a strict contract implementation for the currently used response DTOs, **not a general-purpose JSON Schema Draft validator**.

## v1.3.2: peer-agent tool-call validation recovery

A production-like WSL test surfaced `critic: invalid search request` after successfully contacting an LLM. The model supplied valid JSON with a semantically invalid agent action (for `search_passages`, `to` must equal `none`, `source_id` must be empty, and `query` must be nonempty). Previously, **any** invalid peer action terminated the entire investigation.

This release retains Go-side allowlists and fail-closed execution, but changes the **research orchestration behavior**:

1. Rejected peer actions are never executed, and are recorded as `invalid_move_rejected` trace events.
2. The same agent gets **one** correction opportunity, supplied with the validation error and explicit field requirements. The retry counts against the model-call budget.
3. If the correction is still invalid, the turn is skipped. When applicable, the guarded Analyst is asked to assess already-verified evidence; no unauthorised tool call is made.
4. Every decision remains gated through the normal verified evidence -> JEV -> immutable narration pipeline.

This is *not* an automatic change to the model's verdict and does not bypass provenance checks. Tests prove the invalid Critic action is rejected, the Critic can correct its action, and the narrator can still reopen research and trigger a second JEV decision.

For AlmaLinux WSL installations running the older checkout, copy only the two changed files from this release into `internal/multiagent/`, run `GOPROXY=off go test ./...`, and rebuild the executable. Do **not** overwrite your existing `.env` or `state/` directory.

See [test report](docs/TEST_REPORT_V1_3_2.md). The local test suite uses mock provider responses; actual Mistral/OpenRouter/TypeSafe outcomes still depend on their APIs and credentials.

### v1.3.3-rc1 note: queue exhaustion and safe finalization

When all autonomous peer messages are exhausted, the bounded router now schedules a guaranteed policy-checked Analyst/JEV decision and Narrator report instead of relying on a deduplicated peer message that could be discarded. See `docs/TEST_REPORT_V1_3_3.md`. This is an offline-tested hotfix, not a live-provider certification.
