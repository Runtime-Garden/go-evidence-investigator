package decisionflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"crypto/sha256"
	"encoding/hex"
	"example.com/evidence-investigator/internal/assessment"
	"example.com/evidence-investigator/internal/investigator"
)

func fixture() []investigator.Passage {
	q := "The treatment lowered blood pressure in the synthetic study."
	h := sha256.Sum256([]byte(q))
	return []investigator.Passage{{SourceID: "synthetic-demo:1:0", Title: "Synthetic study", Quote: q, SHA256: hex.EncodeToString(h[:])}}
}
func responseJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func mockFlow(t *testing.T, jevChoice string, confidence float64, narratorVerdict string) (*Flow, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/systemone" {
			if r.Header.Get("Authorization") != "Bearer test-jev-token" {
				t.Error("missing JEV auth")
			}
			var req struct {
				Model string `json:"model"`
				State struct {
					Claim    string    `json:"claim"`
					Findings []Finding `json:"findings"`
				} `json:"state"`
				Questions map[string]json.RawMessage `json:"questions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.State.Claim == "" || len(req.State.Findings) != 1 {
				t.Error("missing structured state")
			}
			if len(req.Questions) != 3 {
				t.Error("missing typed questions")
			}
			probs := map[string]float64{"SUPPORTED": 0.02, "REFUTED": 0.02, "INSUFFICIENT_EVIDENCE": 0.02, "REQUIRES_REVIEW": 0.94}
			if jevChoice == "SUPPORTED" {
				probs = map[string]float64{"SUPPORTED": 0.94, "REFUTED": 0.02, "INSUFFICIENT_EVIDENCE": 0.02, "REQUIRES_REVIEW": 0.02}
			}
			responseJSON(w, map[string]any{"model": "jev-mock", "answers": map[string]any{
				"verdict":          map[string]any{"type": "choice", "choice": jevChoice, "confidence": confidence, "probabilities": probs},
				"needs_review":     map[string]any{"type": "noul", "noul": 0.12},
				"evidence_quality": map[string]any{"type": "score", "score": 2.2, "confidence": 0.92, "probabilities": map[string]float64{"0": 0.1, "1": 0.1, "2": 0.3, "3": 0.5}, "legend": map[string]string{"0": "none", "1": "indirect", "2": "partial", "3": "direct"}},
			}, "usage": map[string]int{"input_tokens": 123, "output_tokens": 19}})
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			var request struct {
				ResponseFormat struct {
					Type       string `json:"type"`
					JSONSchema struct {
						Strict bool           `json:"strict"`
						Schema map[string]any `json:"schema"`
					} `json:"json_schema"`
				} `json:"response_format"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.ResponseFormat.Type != "json_schema" || !request.ResponseFormat.JSONSchema.Strict {
				t.Error("no strict schema request")
			}
			i := calls.Add(1)
			content := `{"findings":[{"source_id":"synthetic-demo:1:0","quote":"lowered blood pressure","relation":"SUPPORTS","reason":"The synthetic sentence reports reduced blood pressure."}]}`
			if i == 2 {
				content = fmt.Sprintf(`{"verdict":%q,"summary":"The provisional synthetic study evidence appears consistent with the claim, but requires human review.","citations":[{"source_id":"synthetic-demo:1:0","quote":"lowered blood pressure"}]}`, narratorVerdict)
			}
			responseJSON(w, map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Flow{LLM: &assessment.Client{Model: "test-llm", BaseURL: srv.URL + "/v1", HTTP: srv.Client()}, JEV: &JevClient{APIKey: "test-jev-token", Model: "jev-latest", Endpoint: srv.URL + "/v1/systemone", HTTP: srv.Client()}}, &calls
}

func TestFlowUsesJEVThenNarrator(t *testing.T) {
	flow, calls := mockFlow(t, "SUPPORTED", 0.95, "SUPPORTED")
	out, err := flow.Run(context.Background(), "The treatment lowered blood pressure", fixture())
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two LLM calls, got %d", calls.Load())
	}
	if out.Decision.JEVChoice != "SUPPORTED" || out.Decision.Verdict != "SUPPORTED" || out.Narrative.Verdict != "SUPPORTED" {
		t.Fatal(out)
	}
	if out.Decision.Model != "jev-mock" || !out.Decision.ReviewRequired {
		t.Fatal(out.Decision)
	}
}
func TestLowConfidenceIsNotAutomaticVerdict(t *testing.T) {
	flow, _ := mockFlow(t, "SUPPORTED", 0.55, "REQUIRES_REVIEW")
	out, err := flow.Run(context.Background(), "The treatment lowered blood pressure", fixture())
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision.Verdict != "REQUIRES_REVIEW" || out.Decision.JEVChoice != "SUPPORTED" {
		t.Fatal(out.Decision)
	}
}
func TestNarratorCannotChangeVerdict(t *testing.T) {
	flow, _ := mockFlow(t, "SUPPORTED", 0.95, "REFUTED")
	_, err := flow.Run(context.Background(), "The treatment lowered blood pressure", fixture())
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected reject, got %v", err)
	}
}
func TestRejectFabricatedAnalystQuote(t *testing.T) {
	a := Analysis{Findings: []Finding{{SourceID: "synthetic-demo:1:0", Quote: "nonexistent study result", Relation: "SUPPORTS", Reason: "made up"}}}
	if ValidateAnalysis(a, fixture()) == nil {
		t.Fatal("fabricated source accepted")
	}
}
func TestRejectJevInvalidDistribution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		responseJSON(w, map[string]any{"model": "jev-mock", "answers": map[string]any{
			"verdict":          map[string]any{"type": "choice", "choice": "SUPPORTED", "confidence": 1.0, "probabilities": map[string]float64{"SUPPORTED": 0.9}},
			"needs_review":     map[string]any{"type": "noul", "noul": 0.1},
			"evidence_quality": map[string]any{"type": "score", "score": 2},
		}})
	}))
	defer srv.Close()
	client := JevClient{Model: "jev-latest", APIKey: "t", Endpoint: srv.URL, HTTP: srv.Client()}
	_, err := client.Decide(context.Background(), map[string]any{"claim": "synthetic"})
	if err == nil {
		t.Fatal("accepted malformed JEV distribution")
	}
}
func TestRemoteJevRequiresHTTPS(t *testing.T) {
	c := JevClient{Model: "jev-latest", APIKey: "t", Endpoint: "http://example.com/v1/systemone"}
	if c.Validate() == nil {
		t.Fatal("insecure JEV host")
	}
}
