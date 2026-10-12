# Evidence Investigator — v1.3.1-rc1 Test Report

Date: 2026-10-12. Environment: Go 1.23.2 on Linux, with Go module network access disabled (`GOPROXY=off GOSUMDB=off`).

| Check | Result |
|---|---|
| `go test ./...` | PASS (all tested Go packages) |
| `go test -race ./...` | PASS |
| `go vet ./...` | PASS |
| `go build -trimpath ./cmd/evidence-investigator` | PASS |
| Synthetic retrieval eval: `-mode=eval -data=data/demo -k=5` | PASS, 5/5 document hit; **not SciFact quality** |
| Local REST smoke (`/healthz`, `/api/info`) | PASS, fresh loopback instance using the built binary |
| Strict schema contract tests | PASS including extra/absent/null/invalid type/enum, duplicates, recursion, oversized payload, overflow, exponent bounds, atomic target, immutable SchemaFor results |
| Mock LLM/JEV and agent reentry tests from v1.3 | PASS |
| Actual LLM API call | NOT RUN — credentials unavailable |
| Actual TypeSafe JEV API call | NOT RUN — credentials unavailable |
| SciFact end-to-end benchmark | NOT RUN — actual corpus/provider access unavailable |
| Docker image build | NOT RUN — Docker CLI/daemon absent in this environment |
| VPS deployment / browser E2E | NOT RUN — no remote access |

**Scope:** The validator implements a limited, project-owned typed contract, not arbitrary JSON Schema Draft compliance. This patch removes the third-party dependency without weakening the currently supported output DTO checks. It does not establish model accuracy, citation soundness beyond configured guards, production readiness, or correct behavior for all providers.
