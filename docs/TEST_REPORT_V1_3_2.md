# Evidence Investigator v1.3.2-rc1 — Local Verification

Date: 12 October 2026
Status: Implementation candidate; no live provider or VPS claim.

## User-observed reproduction

AlmaLinux 10 WSL, Mistral-compatible LLM provider configured, synthetic demo corpus:
- server listening and authenticated UI worked
- workflow reached Critic after prior LLM agent calls
- `critic: invalid search request` rejected by Go's semantic tool-access validator

Original code returned an error on the first invalid structured peer action.

## Changes

- Agent prompts clarify required `search_passages` argument constraints.
- Failed validation adds an `invalid_move_rejected` event.
- A single correction attempt is issued to the **same** agent, subject to model call budget.
- A second invalid move is skipped and safely redirects eligible research peers to the Analyst using already verified evidence.
- Validator still rejects unauthorized commands and no invalid tool ever executes.

## Tests performed in isolated authoring environment

| Test | Result |
| --- | --- |
| `GOPROXY=off go test ./...` | PASS |
| `GOPROXY=off go test -race ./...` | PASS |
| `GOPROXY=off go vet ./...` | PASS |
| `GOPROXY=off go build -o /tmp/evidence-investigator-132 ./cmd/evidence-investigator` | PASS |
| `/tmp/evidence-investigator-132 -mode=eval -data=data/demo -k=5` | 5/5 synthetic document hits |
| Mock Critic issues an invalid directed search, receives correction and investigation still completes | PASS |
| Mock Narrator reopens research and invokes a second JEV decision | PASS |
| ZIP integrity | Checked separately during packaging |

## Not verified

- Live requests from the new release to Mistral/OpenRouter and decision endpoint
- Behavior of every possible provider-produced invalid move
- SciFact evaluation on real corpus
- Docker build, WSL browser UI, VPS deployment

Production deployments should monitor `invalid_move_rejected` in agent traces and retain model budgets. The correction does not guarantee that all provider-produced JSON is semantically valid.
