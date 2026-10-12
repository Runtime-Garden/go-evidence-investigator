package assessment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type dummyOutput struct {
	Label string `json:"label" enum:"OK,NOT_OK"`
}

func TestProviderReceivesStrictSchemaAndValidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Strict bool           `json:"strict"`
					Schema map[string]any `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.ResponseFormat.Type != "json_schema" || !req.ResponseFormat.JSONSchema.Strict {
			t.Error("strict schema missing")
		}
		obj := req.ResponseFormat.JSONSchema.Schema
		if obj["additionalProperties"] != false {
			t.Error("must reject unknown fields")
		}
		label := obj["properties"].(map[string]any)["label"].(map[string]any)
		if len(label["enum"].([]any)) != 2 {
			t.Error("enum not in wire schema")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"label\":\"OK\"}"}}]}`))
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, Model: "mock", HTTP: server.Client()}
	var out dummyOutput
	if err := c.CompleteJSON(context.Background(), "Return JSON", map[string]any{"claim": "test"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Label != "OK" {
		t.Fatal(out)
	}
}

func TestProviderRejectsRefusalAndIncomplete(t *testing.T) {
	for _, message := range []string{
		`{"choices":[{"finish_reason":"stop","message":{"refusal":"Not allowed"}}]}`,
		`{"choices":[{"finish_reason":"length","message":{"content":"{\"label\":\"OK\"}"}}]}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(message))
		}))
		c := &Client{BaseURL: server.URL, Model: "mock", HTTP: server.Client()}
		var out dummyOutput
		if err := c.CompleteJSON(context.Background(), "Return JSON", nil, &out); err == nil {
			t.Fatal("refusal or truncation accepted")
		}
		server.Close()
	}
}
func TestJSONModeStillValidates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		format := body["response_format"].(map[string]any)
		if format["type"] != "json_object" {
			t.Error(format)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"label\":\"INVALID\"}"}}]}`))
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, Model: "mock", ResponseFormat: "json_object", HTTP: server.Client()}
	var out dummyOutput
	if err := c.CompleteJSON(context.Background(), "Return JSON", nil, &out); err == nil || !strings.Contains(err.Error(), "Schema") {
		t.Fatalf("local enum validation missing: %v", err)
	}
}
