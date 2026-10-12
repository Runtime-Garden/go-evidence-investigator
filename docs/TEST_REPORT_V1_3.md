# v1.3.0-rc1 verification log — 2026-10-12

## Scope

Event-driven agent messages, cross-agent delegation, narrator-triggered re-investigation, evidence-revision invalidation, source/tool permission checks, JEV re-decision, existing API and casebook backward compatibility.

## Completed checks

| Check | Result | Evidence / limitation |
|---|---|---|
| Go formatting (`gofmt`) | Passed | Runtime, integration tests, server changes |
| Go syntax/build + `go test ./...` | Passed **with temporary schema-module stand-in** | Local stand-in substituted for unavailable `jsonschema/v6` dependency **in a disposable test copy only**. Tests DO NOT certify real library compatibility. |
| Race detector | Passed **with stand-in** | `go test -race ./internal/multiagent` |
| `go vet ./...` | Passed **with stand-in** | No reported vet diagnostics |
| Narrator -> Researcher -> fresh source -> JEV again | Passed **with stand-in** | `TestNarratorReopensResearchAndInvalidatesDecision`, mocked HTTP clients |
| Role/tool boundary rejections | Passed **with stand-in** | `TestAgentMessagePermissions` |
| Existing API, casebook, JEV mock regression suite | Passed **with stand-in** | `go test ./...` |
| JS/shell syntax | Passed | `node --check`, `bash -n` |
| Real Go dependency download | **Blocked** | DNS in authoring environment cannot reach `proxy.golang.org`; actual `jsonschema/v6` compatibility remains unverified |
| Docker image build | Not run | No Docker runtime accessible |
| Live JEV and LLM providers | Not run | API keys not configured in this environment |
| Real SciFact agentic benchmark | Not run | Actual dataset/inference billing and provider calls required |
| VPS deployment | Not run | No remote authorization / server access |

## Reproduction on a networked development system

```bash
go mod download
go test ./...
go test -race ./...
go vet ./...
docker compose build
```

Perform a controlled `use_agents=true` API test using configured mock providers, followed by real LLM + TypeSafe JEV under spending limits. Review event traces and persist a test case. Existing `use_jev` and retrieval-only modes should also be regression-tested. Do not label the service as production-deployed or model-validated until live tests and human review are completed.

No mock credentials or local schema-module stand-in have been included in the release ZIP.
