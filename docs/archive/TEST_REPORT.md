# Local Test Report — Evidence Investigator v0.1.0

**Date:** 12 October 2026  
**Environment:** ephemeral Linux container with Go 1.23.2 and Python 3.13.5  
**Scope:** repository source, synthetic data and mock LLM only; public SciFact data and VPS were not accessed.

| Test | Status | Comment |
|---|---|---|
| `go test ./...` | PASS | Unit and HTTP handler test suites |
| `go test -race ./...` | PASS | Go race detector |
| `go vet ./...` | PASS | Static analysis |
| `python3 -m py_compile scripts/fetch_scifact.py` | PASS | Python syntax verification |
| Local archive adapter smoke | PASS | Extracted a synthetic tar.gz with the same JSONL structure and verified file contents |
| `go build -trimpath ...` | PASS | Host binary build |
| `GET /healthz` | PASS | HTTP 200, 5 synthetic documents loaded on 127.0.0.1:18976 |
| `POST /api/investigate` | PASS | HTTP 200, NOT_ASSESSED, 3 retrieved passages, first ID synthetic-demo:100002:0 |
| `GET /` | PASS | HTTP 200, embedded static UI |
| Toy document recall@5 | 5/5 | Synthetic fixture only; **not meaningful scientific performance** |
| Real SciFact retrieval evaluation | NOT RUN | Must download official dataset and benchmark |
| Live LLM/ADK execution | NOT RUN | LLM mocked in tests; ADK not implemented |
| Docker image/Compose build | NOT RUN | Docker daemon unavailable in this environment |
| Public VPS deployment/security test | NOT RUN | No changes performed to the user's infrastructure |

Evaluation status is intentionally separated from deployment status. No public address or GitHub repository is claimed to exist.
