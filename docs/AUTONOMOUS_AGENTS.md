# Autonomous peer-to-peer agent runtime (v1.3 candidate)

## Architectural change

The previous bounded round-based Planner -> parallel Researchers -> Skeptic -> Analyst -> JEV -> Narrator workflow has been **replaced** with a typed **event-driven message router** in `internal/multiagent/runtime.go`.

There are five LLM-controlled roles: `planner`, `researcher`, `critic`, `analyst`, and `narrator`. They share a source-verified evidence blackboard, receive message envelopes, and choose **one** action per message. They may delegate to **any other agent** (but not themselves), including to an agent that ran earlier. A `narrator` may reopen research instead of publishing a report. No fixed stage transitions are used to route research work.

### Message contract

All agent control outputs have one strict Go-typed JSON Schema contract:

```json
{"kind":"delegate","to":"researcher","query":"","source_id":"","message":"Check the alternative greenhouse temperature explanation"}
```

Valid `kind`: `delegate`, `search_passages`, `inspect_passage`, `expand_document`, `decide`, `report`, `finish`.

- `delegate`: any role -> any **other** role, with a bounded task message.
- `search_passages`: any role except narrator, using a local corpus query.
- `inspect_passage` / `expand_document`: any role except narrator; requires an observed source ID.
- `decide`: analyst only; invokes strict `Flow.RunWithNarrator(..., false)` to construct verified findings, then sends a typed request to JEV. The LLM analyst **cannot choose JEV's verdict**.
- `report`: narrator only; composes a verified human-readable explanation of the **current** JEV/policy decision via `Flow.Narrate`. Narrative content cannot alter the verdict or invent citation text.
- `finish`: no further action from that agent for this message.

The router reads LLM commands as **untrusted proposals**. Go validates message recipients, tool arguments, observed source permissions, decision/publishing permissions, and exact source integrity **before** executing any proposal. There is no shell, arbitrary HTTP, file-write, external crawling, or side-effect tool in the registry. Corpus data and peer messages are passed as untrusted JSON data rather than system instructions.

### Example non-linear conversation

```text
Researcher -> Analyst: "Enough evidence to assess?"
Analyst -> JEV: typed verified evidence, provisional decision
JEV -> Narrator: locked decision for this source revision
Narrator -> Researcher: "Find the missing control-group comparison"
Researcher -> corpus search -> fresh source / evidence revision += 1
New source -> Critic: "Challenge the new citation"
New source -> Analyst: "Reevaluate the evidence"
Analyst -> JEV: new source revision (old decision is invalid)
JEV -> Narrator: updated typed decision
Narrator -> report: validated summary, current JEV decision
```

**Critical invariant:** Source revisions make old JEV decisions stale. A narrator cannot publish a decision computed on older evidence; new sources require a fresh verified analysis and JEV decision. Every report remains pending human review.

### Guardrails and termination

"Autonomous" does not mean an infinite, unbounded process. Unlimited loops can burn API credit, duplicate tool calls, stall a server request, and get stuck passing messages in circles. Defaults, capped by Go code:

| Limit | Default | Rationale |
|---|---:|---|
| Agent turns | 24 | Protect against self-triggering loops |
| LLM calls | 38 | Bound provider spend; reserve final analysis/narration |
| Local read-only tool calls | 14 | Avoid repeated corpus scans |
| JEV decisions | 3 | Allow *at most* two evidence-driven revisions |
| Queued messages | 48 | Bound agent-to-agent fanout |
| No-progress actions | 8 | Converge when tasks do not add new verified sources |
| Request deadline | 150s | Synchronous API resource bound |
| Evidence passages | 10 | Bounded provider state and citations |

Once limits approach, Go stops exploratory chatter and (if sufficient verified evidence is present) requests a final typed JEV evaluation and narrator report. If the JEV/API/schema check cannot succeed, **the request fails closed** with no published verdict. The system does not falsely assert that a globally best answer was found.

The dispatcher is intentionally single-writer (deterministic ledger revision order). This is *event-driven coordination*, not a fixed-sequence workflow, but it is not a distributed swarm or concurrent actor processes. Independent messages are processed one by one. Provider calls still use direct `net/http`/JSON, without an AI SDK.

### API

`POST /api/investigate` with:

```json
{"claim":"In simulated tomato greenhouse studies, cooler conditions reduce fruit yield.","top_k":5,"use_agents":true}
```

Do not combine `use_agents` with `use_model` or `use_jev` (mutually exclusive execution modes). Responses include `agent_trace` containing role, message target, source IDs, evidence revision, JEV call count and stop reason. Durable case-review data remains in the existing `casebook` store.

### Verification and operational limitations

- Mock unit/integration tests have been **written** including narrator -> researcher reentry -> source revision -> repeated JEV, unsafe tool/role rejections.
- **v1.3.1 update:** the external schema module was removed. All Go tests, race tests, vet, build and synthetic retrieval evaluation passed offline (`GOPROXY=off`). Peer-to-peer behavior is still verified only with mock LLM/JEV services, not live providers.
- Actual TypeSafe JEV, a live LLM, SciFact full benchmark, Docker image build, and VPS deployment remain unverified.
- Model-driven agent communication can still be wrong, inefficient, or semantically misleading. Even with JSON Schema, exact quote checking and a deterministic policy gate, external human assessment is required.
