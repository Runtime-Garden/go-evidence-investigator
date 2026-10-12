# v0.2 implementation & test report

Date: 2026-10-12

- Implemented JEV HTTP adapter with TypeSafe's `POST /v1/systemone` state/questions/answers schema.
- Implemented validation-first LLM analyst and final LLM narrator, separated by JEV and Go safety policy.
- Implemented optional `use_jev` mode, preserving existing `use_model` and retrieval-only API compatibility.
- Added UI toggle and readable JEV decision metadata.
- Added mock tests for successful JEV flow, low-confidence gate, altered verdict rejection, fabricated evidence rejection, malformed probability distribution and insecure endpoints.
- Ran `go test -count=1 ./...`, `go test -race ./...`, and `go vet ./...` locally; all passed.
- **Not executed**: actual TypeSafe JEV calls, actual LLM provider calls, SciFact evaluation, Docker image build, VPS deployment, browser visual test and production security audit.

Result: **Implemented and mock-tested prototype**, not production-ready and not yet experimentally validated for scientific claims.

## Additional smoke results

- `go build -trimpath ./cmd/evidence-investigator`: passed using local Go 1.23.2.
- Started local app on `127.0.0.1:18087` with synthetic corpus: `/healthz` returned `200` with five documents.
- Retrieval-only `/api/investigate`: returned `NOT_ASSESSED` and five retrieved passages.
- With JEV not configured, `use_jev=true` returned HTTP `400` (`JEV workflow is not configured`), rather than creating a fake model decision.
- Docker tooling was unavailable in the execution environment, so image/Compose runtime validation was not performed.
