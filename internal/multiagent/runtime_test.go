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

func TestAgentMessagePermissions(t *testing.T) {
	valid := Move{Kind: "delegate", To: "researcher", Message: "Re-check the source"}
	if err := validateMove(narrator, valid, nil); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		role     string
		move     Move
		observed map[string]bool
	}{
		{researcher, Move{Kind: "decide", To: "none"}, nil},
		{critic, Move{Kind: "report", To: "none"}, nil},
		{narrator, Move{Kind: "search_passages", To: "none", Query: "evidence"}, nil},
		{planner, Move{Kind: "delegate", To: planner, Message: "loop"}, nil},
		{researcher, Move{Kind: "inspect_passage", To: "none", SourceID: "unseen"}, map[string]bool{}},
		{researcher, Move{Kind: "search_passages", To: "none", Query: ""}, nil},
	}
	for _, test := range tests {
		if err := validateMove(test.role, test.move, test.observed); err == nil {
			t.Fatalf("accepted unsafe action: %+v", test.move)
		}
	}
}

// A JEV decision is reached, then the final narrator sends a *backwards*
// task to researcher, which finds a new source and forces a second JEV run.
func TestNarratorReopensResearchAndInvalidatesDecision(t *testing.T) {
	engine, err := investigator.Load(filepath.Join("..", "..", "data", "demo", "corpus.jsonl"), "", "synthetic-demo")
	if err != nil {
		t.Fatal(err)
	}
	var jevCalls, narratorReviews, criticCalls atomic.Int32
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
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		if len(req.Messages) != 2 {
			t.Error("missing system and user roles")
			return
		}
		prompt := req.Messages[0].Content
		input := map[string]json.RawMessage{}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Error(err)
			return
		}
		var response any
		switch {
		case strings.Contains(prompt, "You are PLANNER"):
			response = Move{Kind: "delegate", To: analyst, Message: "Evaluate the initial evidence", Query: "", SourceID: ""}
		case strings.Contains(prompt, "You are RESEARCHER"):
			var m Message
			_ = json.Unmarshal(input["message"], &m)
			if strings.Contains(m.Topic, "additional") {
				response = Move{Kind: "search_passages", To: "none", Query: "four simulated trials reduced tomato fruit yield", Message: "Look for the second experiment sentence"}
			} else {
				response = Move{Kind: "finish", To: "none", Message: "Waiting for a targeted request"}
			}
		case strings.Contains(prompt, "You are CRITIC"):
			if criticCalls.Add(1) == 1 {
				// Simulates a real structured-JSON output with semantically invalid
				// search arguments: target must be "none" for search_passages.
				response = Move{Kind: "search_passages", To: "researcher", Query: "tomato yield"}
			} else {
				response = Move{Kind: "finish", To: "none", Message: "No additional counterevidence"}
			}
		case strings.Contains(prompt, "You are ANALYST"):
			response = Move{Kind: "decide", To: "none", Message: "Let JEV assess sources"}
		case strings.Contains(prompt, "FINAL REPORT REVIEWER"):
			if narratorReviews.Add(1) == 1 {
				response = Move{Kind: "delegate", To: researcher, Message: "Find additional experimental evidence"}
			} else {
				response = Move{Kind: "report", To: "none", Message: "Report the locked JEV outcome"}
			}
		case strings.Contains(prompt, "evidence analyst, NOT the final decision-maker"):
			var excerpts []struct {
				SourceID string `json:"source_id"`
				Quote    string `json:"quote"`
			}
			_ = json.Unmarshal(input["passages"], &excerpts)
			findings := []decisionflow.Finding{}
			for _, e := range excerpts {
				quote := e.Quote
				if len(quote) > 35 {
					quote = quote[:35]
				}
				findings = append(findings, decisionflow.Finding{SourceID: e.SourceID, Quote: quote, Relation: "SUPPORTS", Reason: "The synthetic passage explicitly supports the limited greenhouse claim."})
			}
			response = decisionflow.Analysis{Findings: findings}
		case strings.Contains(prompt, "final report writer, NEVER the decision-maker"):
			var excerpts []struct {
				SourceID string `json:"source_id"`
				Quote    string `json:"quote"`
			}
			_ = json.Unmarshal(input["sources"], &excerpts)
			response = decisionflow.Narrative{Verdict: "SUPPORTED", Summary: "Only the simulated greenhouse trials were evaluated. The provisional result requires a human reviewer.", Citations: []assessment.Citation{{SourceID: excerpts[0].SourceID, Quote: excerpts[0].Quote[:20]}}}
		default:
			t.Errorf("unrecognized prompt %s", prompt)
			return
		}
		content, _ := json.Marshal(response)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": string(content)}, "finish_reason": "stop"}}})
	}))
	defer svc.Close()
	model := &assessment.Client{Model: "mock", BaseURL: svc.URL + "/v1", HTTP: svc.Client()}
	flow := &decisionflow.Flow{LLM: model, JEV: &decisionflow.JevClient{APIKey: "test", Model: "mock-jev", Endpoint: svc.URL + "/v1/systemone", HTTP: svc.Client()}}
	rt := Runtime{Engine: engine, Model: model, Flow: flow, Limits: DefaultLimits()}
	result, err := rt.Run(context.Background(), "Tomato plants exposed to 12 degrees Celsius develop fewer fruits than at 22 degrees Celsius", 1)
	if err != nil {
		t.Fatal(err)
	}
	if jevCalls.Load() != 2 {
		t.Fatalf("expected repeat JEV decision after new evidence, got %d", jevCalls.Load())
	}
	if result.Report.Decision.Verdict != "SUPPORTED" || result.Report.Narrative.Verdict != "SUPPORTED" {
		t.Fatal(result.Report)
	}
	if narratorReviews.Load() != 2 {
		t.Fatalf("expected narrator twice, got %d", narratorReviews.Load())
	}
	if criticCalls.Load() < 2 {
		t.Fatal("critic was not allowed to retry after invalid search arguments")
	}
	sawBackwards := false
	sawRejected := false
	for _, e := range result.Events {
		if e.Agent == critic && e.Action == "invalid_move_rejected" {
			sawRejected = true
		}
		if e.Agent == narrator && e.Action == "delegate" && e.To == researcher {
			sawBackwards = true
		}
	}
	if !sawBackwards {
		t.Fatal("narrator did not send work backwards to researcher")
	}
	if !sawRejected {
		t.Fatal("invalid critic search was not recorded in the audit trace")
	}
	if len(result.Evidence) < 2 {
		t.Fatal("researcher did not expand evidence")
	}
}
