# Evidence Investigator v1.0.0-rc1 — local verification

**Date:** 2026-10-12

**Environment:** Local Linux workspace with Go 1.23.2 and Node.js v22.16.0. Docker binary, TypeSafe key, LLM key and VPS SSH connection were not available.

## Executed successfully

| Verification | Outcome |
|---|---|
| `go test ./...` | PASS: all Go package tests, including provider mocks, structured JEV workflow, citation guards, case integrity, reviewer authorization, benchmark experiment and retry behavior |
| `go test -race ./...` | PASS: no race detected in exercised test cases |
| `go vet ./...` | PASS |
| `go build -trimpath ./cmd/evidence-investigator` | PASS |
| `node --check internal/server/web/app.js` | PASS: JavaScript syntax |
| `sh -n deploy/verify.sh` and `sh -n deploy/backup.sh` | PASS: shell syntax |
| `go run ... -mode=eval -data=data/demo -k=5` | PASS: synthetic sample only — 5/5 document hits; no generalization claim |
| Live local HTTP smoke with separate auth tokens and local synthetic corpus | PASS: missing token returns 401; authorized info returns 200, investigation returns 200 and creates case ID; health returns 200 |

## Corrections after first local run

- Removed an unused import found during compile/test.
- Added strict Jev score distribution/legend/confidence validation, aligned mock distributions to mathematically consistent expected scores and reran complete Go suite.
- Retained the v0.2 test fixtures so v1 behavior remains backward-compatible for unconfigured demo mode.

## Not executed / blocked

1. **Real Jev System One inference:** API key, quota and provider access not shared.
2. **Real external LLM provider:** API credentials and model selection not shared.
3. **SciFact public download:** remote dataset transfer not available in this local environment. Native downloader and command included.
4. **Paid SciFact LLM-vs-Jev benchmark:** needs real dataset and both providers; only fixture mock test ran.
5. **Docker image build and Compose start:** Docker CLI absent.
6. **Browser E2E including a review session:** no browser automation executed; JS syntax, endpoint and mocked review tests passed.
7. **VPS, Cloudflare, NPM and public GitHub publication:** no credentials/permissions or verified target in session. No deployment or public launch claimed.

**Conclusion:** Implementation and local regression test candidate. Not yet a fully verified production deployment or demonstrated scientific result. Follow `RUNBOOK.md` and `BENCHMARK.md` before marking as externally validated.

## v1.1 regression status (12 October 2026)

New schema-contract tests added in `internal/structured/schema_test.go` and `internal/assessment/structured_test.go`, and the JEV mock workflow now asserts strict response format fields. These **new tests were not executed** because this environment cannot reach `proxy.golang.org` to fetch the newly added Go library. The previously recorded v1.0 passes apply to v1.0 source only, not to the revised v1.1 dependency graph. Run the commands in README on a network-enabled developer workstation or CI before promotion.
