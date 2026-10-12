package multiagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"example.com/evidence-investigator/internal/assessment"
	"example.com/evidence-investigator/internal/decisionflow"
	"example.com/evidence-investigator/internal/investigator"
)

// An agent may legally finish without requesting JEV. If all research agents do
// that, the router must still assess the bounded verified evidence, rather than
// re-queue an already-seen analyst message forever or publish an invented verdict.
func TestQueueExhaustionTriggersBoundedJEVFinalization(t *testing.T) {
	engine, err := investigator.Load(filepath.Join("..", "..", "data", "demo", "corpus.jsonl"), "", "synthetic-demo")
	if err != nil {
		t.Fatal(err)
	}
	var jevCalls, llmMoves atomic.Int32
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/systemone" {
			jevCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"model": "mock-jev", "answers": map[string]any{
				"verdict":          map[string]any{"type": "choice", "choice": "SUPPORTED", "confidence": 0.94, "probabilities": map[string]float64{"SUPPORTED": 0.94, "REFUTED": 0.02, "INSUFFICIENT_EVIDENCE": 0.02, "REQUIRES_REVIEW": 0.02}},
				"needs_review":     map[string]any{"type": "noul", "noul": 0.12},
				"evidence_quality": map[string]any{"type": "score", "score": 2.2, "confidence": 0.92, "probabilities": map[string]float64{"0": 0.1, "1": 0.1, "2": 0.3, "3": 0.5}, "legend": map[string]string{"0": "none", "1": "indirect", "2": "partial", "3": "direct"}},
			}})
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 2 {
			t.Errorf("bad mock LLM request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		prompt := req.Messages[0].Content
		input := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Errorf("bad prompt input: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var response any
		switch {
		case strings.Contains(prompt, "autonomous research agent in a message-passing team"):
			llmMoves.Add(1)
			response = Move{Kind: "finish", To: "none", Message: "Nothing else to add"}
		case strings.Contains(prompt, "evidence analyst, NOT the final decision-maker"):
			var passages []struct {
				SourceID string `json:"source_id"`
				Quote    string `json:"quote"`
			}
			if err := json.Unmarshal(input["passages"], &passages); err != nil {
				t.Error(err)
			}
			findings := make([]decisionflow.Finding, 0, len(passages))
			for _, p := range passages {
				findings = append(findings, decisionflow.Finding{SourceID: p.SourceID, Quote: p.Quote, Relation: "SUPPORTS", Reason: "Passage explicitly supports the synthetic claim."})
			}
			response = decisionflow.Analysis{Findings: findings}
		case strings.Contains(prompt, "final report writer, NEVER the decision-maker"):
			var decision struct {
				Verdict string `json:"verdict"`
			}
			_ = json.Unmarshal(input["decision"], &decision)
			var sources []struct {
				SourceID string `json:"source_id"`
				Quote    string `json:"quote"`
			}
			_ = json.Unmarshal(input["sources"], &sources)
			response = decisionflow.Narrative{Verdict: decision.Verdict, Summary: "Provisional result on synthetic evidence, awaiting human review.", Citations: []assessment.Citation{{SourceID: sources[0].SourceID, Quote: sources[0].Quote}}}
		default:
			t.Errorf("unexpected mock prompt: %.80s", prompt)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		content, _ := json.Marshal(response)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": string(content)}, "finish_reason": "stop"}}})
	}))
	defer svc.Close()
	model := &assessment.Client{Model: "mock", BaseURL: svc.URL + "/v1", HTTP: svc.Client()}
	flow := &decisionflow.Flow{LLM: model, JEV: &decisionflow.JevClient{APIKey: "test", Model: "mock-jev", Endpoint: svc.URL + "/v1/systemone", HTTP: svc.Client()}}
	rt := Runtime{Engine: engine, Model: model, Flow: flow, Limits: DefaultLimits()}
	result, err := rt.Run(context.Background(), "In a simulated greenhouse, lower temperatures reduced tomato fruit yield.", 3)
	if err != nil {
		t.Fatal(err)
	}
	if jevCalls.Load() != 1 || result.DecisionCalls != 1 {
		t.Fatalf("expected one guarded JEV decision, got %d model JEV calls and %d result decisions", jevCalls.Load(), result.DecisionCalls)
	}
	if llmMoves.Load() != 3 {
		t.Fatalf("expected seed planner, researcher and critic to finish, got %d model moves", llmMoves.Load())
	}
	if result.Report.Decision.Verdict != "SUPPORTED" || result.Report.Narrative.Verdict != "SUPPORTED" {
		t.Fatalf("JEV/narrator verdict mismatch: %#v", result.Report)
	}
	if result.StopReason != "queue_exhausted" {
		t.Fatalf("expected queue_exhausted convergence, got %q", result.StopReason)
	}
	var forcedAnalyst, reported bool
	for _, evt := range result.Events {
		if evt.Action == "bounded_finalization" && evt.To == analyst {
			forcedAnalyst = true
		}
		if evt.Action == "publish_verified_report" {
			reported = true
		}
	}
	if !forcedAnalyst || !reported {
		t.Fatalf("missing audit events: forcedAnalyst=%t reported=%t", forcedAnalyst, reported)
	}
}
