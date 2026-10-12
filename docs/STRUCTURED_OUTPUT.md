# Structured Output Contracts — v1.3.1

Evidence Investigator uses **no third-party JSON Schema module and no AI SDK**.
`internal/structured` generates strict JSON Schema from Go DTOs and verifies returned JSON with the same small built-in contract implementation using only the Go standard library (`encoding/json`, `reflect`, `math/big`).

## Request / response path

```text
Typed Go DTO with json / enum tags
  -> SchemaFor generates strict provider-facing JSON Schema
  -> direct HTTP POST /chat/completions (response_format: json_schema, strict=true)
  -> reject refusal, truncation, empty model response
  -> Decode: size bound -> duplicate keys / trailing data -> strict validation
  -> atomic typed decode (out unchanged on failure)
  -> domain validation (verbatim citations, provenance, consistent verdict)
  -> JEV decision and policy -> narrator with the same validation
```

## Supported subset (intentional)

| Rule | Supported |
|---|---|
| JSON object and nested object | Yes |
| Arrays and arrays of objects | Yes |
| Required fields | Every exported DTO field required |
| Disallow additional properties | Yes |
| `string`, `boolean`, `integer`, `number` | Yes |
| String enum (`enum:"A,B"` struct tag) | Yes |
| JSON duplicate key/trailing data rejection | Yes |
| Reject `null` where non-null types are specified | Yes |
| DTO recursion, oversized content, excessive nesting | Rejected |
| Schema `$ref`, `oneOf`, `anyOf`, `pattern`, `format`, regex or arbitrary JSON Schema dialects | **Not implemented** |
| Optional/omittable DTO fields and arbitrary maps | **Not supported for model output** |

A new schema feature must be implemented in both generation and validation with tests before a DTO may use it. **Do not describe this as a full JSON Schema implementation.**

## Provider compatibility

Default mode `LLM_RESPONSE_FORMAT=json_schema` requests server-side `strict` generation. Some OpenAI-compatible providers reject this mode; `LLM_RESPONSE_FORMAT=json_object` is an explicit opt-in fallback. It still performs full local validation of the supported subset; no implicit fallback or silent repair. Neither mode guarantees factual or stylistic correctness. No automatic retry can turn unsupported or semantically wrong evidence into a correct verdict.

## Tests

```bash
GOPROXY=off go test ./...
GOPROXY=off go test -race ./...
GOPROXY=off go vet ./...
GOPROXY=off go build ./cmd/evidence-investigator
```

The local fixture tests cover valid nested DTOs; missing fields; unknown keys; invalid enums and primitive types; `null`; duplicate keys; integer precision, overflow and large exponents; trailing data; recursion and oversized inputs; atomic decode; schema caller mutation; and mock provider `response_format` compatibility. JEV, human approval and scientific citation checks are separate modules.
