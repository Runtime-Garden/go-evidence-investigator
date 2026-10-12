package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"example.com/evidence-investigator/internal/assessment"
	"example.com/evidence-investigator/internal/decisionflow"
	"example.com/evidence-investigator/internal/investigator"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThreeArmBenchmark(t *testing.T) {
	dir := t.TempDir()
	corpus := filepath.Join(dir, "corpus.jsonl")
	claims := filepath.Join(dir, "claims_dev.jsonl")
	_ = os.WriteFile(corpus, []byte(`{"doc_id":17,"title":"Test plants","abstract":["Plants absorb sunlight in the simulated experiment."]}`+"\n"), 0600)
	_ = os.WriteFile(claims, []byte(`{"id":7,"claim":"Plants absorb sunlight","evidence":{"17":[{"label":"SUPPORT","sentences":[0]}]}}`+"\n"), 0600)
	engine, err := investigator.Load(corpus, claims, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "systemone") {
			_ = json.NewEncoder(w).Encode(map[string]any{"model": "jev-mock", "answers": map[string]any{
				"verdict":          map[string]any{"type": "choice", "choice": "SUPPORTED", "confidence": 0.95, "probabilities": map[string]float64{"SUPPORTED": 0.95, "REFUTED": 0.02, "INSUFFICIENT_EVIDENCE": 0.02, "REQUIRES_REVIEW": 0.01}},
				"needs_review":     map[string]any{"type": "noul", "noul": 0.05},
				"evidence_quality": map[string]any{"type": "score", "score": 2.6, "confidence": 0.9, "legend": map[string]string{"0": "none", "1": "indirect", "2": "partial", "3": "direct"}, "probabilities": map[string]float64{"0": 0.0, "1": 0.0, "2": 0.4, "3": 0.6}}}, "usage": map[string]int{"input_tokens": 100, "output_tokens": 10}})
			return
		}
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		content := `{"verdict":"SUPPORTED","explanation":"Supported by the simulated experiment.","citations":[{"source_id":"fixture:17:0","quote":"Plants absorb sunlight"}]}`
		if len(request.Messages) > 0 && strings.Contains(request.Messages[0].Content, "one finding per passage") {
			content = `{"findings":[{"source_id":"fixture:17:0","quote":"Plants absorb sunlight","relation":"SUPPORTS","reason":"Simulated experiment says the plants absorb sunlight."}]}`
		} else if len(request.Messages) > 0 && strings.Contains(request.Messages[0].Content, "final report writer") {
			content = `{"verdict":"SUPPORTED","summary":"A simulated report supports the claim, pending human review.","citations":[{"source_id":"fixture:17:0","quote":"Plants absorb sunlight"}]}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}, "usage": map[string]int{"prompt_tokens": 55, "completion_tokens": 22}})
	}))
	defer server.Close()
	llm := &assessment.Client{BaseURL: server.URL + "/v1", Model: "mock", HTTP: server.Client()}
	flow := &decisionflow.Flow{LLM: llm, JEV: &decisionflow.JevClient{Endpoint: server.URL + "/v1/systemone", APIKey: "fake", Model: "jev-latest", HTTP: server.Client()}}
	var log bytes.Buffer
	summary, err := Run(context.Background(), engine, llm, flow, 5, 1, &log)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Arms) != 3 || summary.Samples != 1 || summary.LLMRequests != 3 || summary.JEVInputTokens != 100 {
		t.Fatalf("bad summary %+v", summary)
	}
	for _, a := range summary.Arms {
		if a.Errors != 0 || a.Correct != 1 {
			t.Fatalf("bad arm %+v details %s", a, log.String())
		}
	}
}
