// Package benchmark provides reproducible, bounded SciFact dev evaluations.
// Gold labels are ONLY accessed here, never by inference code or model prompts.
package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"

	"example.com/evidence-investigator/internal/assessment"
	"example.com/evidence-investigator/internal/decisionflow"
	"example.com/evidence-investigator/internal/investigator"
)

type Arm struct {
	Name           string  `json:"name"`
	Attempted      int     `json:"attempted"`
	Correct        int     `json:"correct"`
	Abstained      int     `json:"abstained"`
	Errors         int     `json:"errors"`
	TotalLatencyMS int64   `json:"total_latency_ms"`
	Accuracy       float64 `json:"accuracy_on_attempted"`
	Coverage       float64 `json:"coverage"`
}
type Result struct {
	Dataset             string               `json:"dataset"`
	Samples             int                  `json:"samples"`
	TopK                int                  `json:"top_k"`
	Retrieval           investigator.Metrics `json:"retrieval"`
	Arms                []Arm                `json:"arms"`
	LLMRequests         int64                `json:"llm_requests"`
	LLMPromptTokens     int64                `json:"llm_prompt_tokens"`
	LLMCompletionTokens int64                `json:"llm_completion_tokens"`
	JEVInputTokens      int64                `json:"jev_input_tokens"`
	JEVOutputTokens     int64                `json:"jev_output_tokens"`
	JEVBrier            float64              `json:"jev_multiclass_brier"`
	JEVECE              float64              `json:"jev_top_choice_ece10"`
	Caution             string               `json:"caution"`
}
type Detail struct {
	ClaimID     int64             `json:"claim_id"`
	Expected    string            `json:"expected"`
	Predictions map[string]string `json:"predictions"`
	Errors      map[string]string `json:"errors,omitempty"`
}

func gold(c investigator.Claim) string {
	labels := map[string]bool{}
	for _, rationales := range c.Evidence {
		for _, v := range rationales {
			labels[v.Label] = true
		}
	}
	if len(labels) != 1 {
		return ""
	}
	if labels["SUPPORT"] {
		return "SUPPORTED"
	}
	if labels["CONTRADICT"] {
		return "REFUTED"
	}
	return ""
}
func Run(ctx context.Context, e *investigator.Engine, llm *assessment.Client, flow *decisionflow.Flow, k, limit int, w io.Writer) (Result, error) {
	if e == nil || llm == nil || flow == nil {
		return Result{}, errors.New("benchmark requires dataset, LLM and JEV")
	}
	if limit < 1 || limit > 50 {
		return Result{}, errors.New("online benchmark limit must be 1..50 (cost guard)")
	}
	if k < 1 || k > 10 {
		return Result{}, errors.New("top-k must be 1..10")
	}
	r := Result{Dataset: e.Dataset, TopK: k, Retrieval: e.Evaluate(k, limit), Caution: "Exploratory SciFact-dev experiment; non-calibrated, not a published SciFact official benchmark. Latencies include remote provider time, excludes retries from cost estimates."}
	names := []string{"llm_only", "llm_plus_jev", "llm_jev_narrator"}
	arms := map[string]*Arm{}
	for _, name := range names {
		arms[name] = &Arm{Name: name}
	}
	beforeCalls, beforePrompt, beforeComp := llm.Calls.Load(), llm.InputTokens.Load(), llm.OutputTokens.Load()
	enc := json.NewEncoder(w)
	brierTotal := 0.0
	var calibBins [10]struct {
		Count      int
		Confidence float64
		Correct    float64
	}
	brierCount := 0
	for _, claim := range e.Claims {
		if r.Samples >= limit {
			break
		}
		expected := gold(claim)
		if expected == "" {
			continue
		}
		r.Samples++
		passages := e.Search(claim.Claim, k)
		detail := Detail{ClaimID: claim.ID, Expected: expected, Predictions: map[string]string{}, Errors: map[string]string{}}
		// Baseline: one LLM verdict (same evidence, same retrieval).
		start := time.Now()
		base, err := llm.Assess(ctx, claim.Claim, passages)
		a := arms["llm_only"]
		a.Attempted++
		a.TotalLatencyMS += time.Since(start).Milliseconds()
		if err != nil {
			a.Errors++
			detail.Errors[a.Name] = err.Error()
		} else {
			detail.Predictions[a.Name] = base.Verdict
			if base.Verdict == expected {
				a.Correct++
			}
			if base.Verdict == "INSUFFICIENT_EVIDENCE" {
				a.Abstained++
			}
		}
		// The three arms share retrieval results. The JEV decision is computed once;
		// the narrator variant reuses it, measuring only its incremental overhead.
		start = time.Now()
		jResult, jErr := flow.RunWithNarrator(ctx, claim.Claim, passages, false)
		jevMS := time.Since(start).Milliseconds()
		a = arms["llm_plus_jev"]
		a.Attempted++
		a.TotalLatencyMS += jevMS
		if jErr != nil {
			a.Errors++
			detail.Errors[a.Name] = jErr.Error()
		} else {
			detail.Predictions[a.Name] = jResult.Decision.Verdict
			r.JEVInputTokens += int64(jResult.Decision.InputTokens)
			r.JEVOutputTokens += int64(jResult.Decision.OutputTokens)
			brier := 0.0
			for label, prob := range jResult.Decision.Probabilities {
				target := 0.0
				if label == expected {
					target = 1
				}
				brier += math.Pow(prob-target, 2)
			}
			brierTotal += brier
			brierCount++
			conf := jResult.Decision.Probabilities[jResult.Decision.JEVChoice]
			bin := int(math.Floor(conf * 10))
			if bin > 9 {
				bin = 9
			}
			calibBins[bin].Count++
			calibBins[bin].Confidence += conf
			if jResult.Decision.JEVChoice == expected {
				calibBins[bin].Correct++
			}
			if jResult.Decision.Verdict == expected {
				a.Correct++
			}
			if jResult.Decision.Verdict == "REQUIRES_REVIEW" || jResult.Decision.Verdict == "INSUFFICIENT_EVIDENCE" {
				a.Abstained++
			}
		}
		a = arms["llm_jev_narrator"]
		a.Attempted++
		a.TotalLatencyMS += jevMS
		if jErr != nil {
			a.Errors++
			detail.Errors[a.Name] = jErr.Error()
		} else {
			start = time.Now()
			_, err = flow.Narrate(ctx, claim.Claim, passages, jResult)
			a.TotalLatencyMS += time.Since(start).Milliseconds()
			if err != nil {
				a.Errors++
				detail.Errors[a.Name] = err.Error()
			} else {
				detail.Predictions[a.Name] = jResult.Decision.Verdict
				if jResult.Decision.Verdict == expected {
					a.Correct++
				}
				if jResult.Decision.Verdict == "REQUIRES_REVIEW" || jResult.Decision.Verdict == "INSUFFICIENT_EVIDENCE" {
					a.Abstained++
				}
			}
		}
		if err := enc.Encode(detail); err != nil {
			return r, err
		}
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
	}
	for _, name := range names {
		a := arms[name]
		if a.Attempted > 0 {
			a.Accuracy = float64(a.Correct) / float64(a.Attempted)
			a.Coverage = float64(a.Attempted-a.Errors-a.Abstained) / float64(a.Attempted)
		}
		r.Arms = append(r.Arms, *a)
	}
	sort.SliceStable(r.Arms, func(i, j int) bool { return strings.Compare(r.Arms[i].Name, r.Arms[j].Name) < 0 })
	if brierCount > 0 {
		r.JEVBrier = brierTotal / float64(brierCount)
		for _, b := range calibBins {
			if b.Count > 0 {
				r.JEVECE += float64(b.Count) / float64(brierCount) * math.Abs(b.Confidence/float64(b.Count)-b.Correct/float64(b.Count))
			}
		}
	}
	r.LLMRequests = llm.Calls.Load() - beforeCalls
	r.LLMPromptTokens = llm.InputTokens.Load() - beforePrompt
	r.LLMCompletionTokens = llm.OutputTokens.Load() - beforeComp
	if r.Samples == 0 {
		return r, fmt.Errorf("no labeled claims in provided evaluation window")
	}
	return r, nil
}
