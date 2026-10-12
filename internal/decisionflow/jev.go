// Package decisionflow implements an evidence-first, JEV-decided investigation.
// JEV judges typed state; application code owns business and safety policy.
package decisionflow

import (
	"context"
	"encoding/json"
	"errors"
	"example.com/evidence-investigator/internal/httpjson"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
)

const defaultEndpoint = "https://api.typesafe.ai/v1/systemone"

type JevClient struct {
	APIKey   string
	Model    string
	Endpoint string
	HTTP     *http.Client
}

func (c *JevClient) Ready() bool { return c != nil && c.APIKey != "" && c.Model != "" }
func (c *JevClient) Validate() error {
	if !c.Ready() {
		return errors.New("JEV_API_KEY and JEV_MODEL are required")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid JEV endpoint")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || (net.ParseIP(u.Hostname()) != nil && net.ParseIP(u.Hostname()).IsLoopback()))) {
		return errors.New("JEV endpoint must use HTTPS or loopback HTTP")
	}
	return nil
}

type jevQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}
type jevRequest struct {
	Model     string                 `json:"model"`
	State     any                    `json:"state"`
	Questions map[string]jevQuestion `json:"questions"`
}
type jevAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}
type jevResponse struct {
	Model   string               `json:"model"`
	Answers map[string]jevAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

const ReviewThreshold = 0.80 // Prototype policy only; NOT empirically calibrated.
const NoulReviewThreshold = 0.65

// Decision stores JEV's unchanged answer and the app's explicit safety gate.
type Decision struct {
	Verdict              string             `json:"verdict"`
	JEVChoice            string             `json:"jev_choice"`
	Confidence           float64            `json:"confidence"`
	Probabilities        map[string]float64 `json:"probabilities"`
	ReviewProbability    float64            `json:"review_probability"`
	EvidenceQualityScore float64            `json:"evidence_quality_score"`
	Model                string             `json:"model"`
	InputTokens          int                `json:"input_tokens"`
	OutputTokens         int                `json:"output_tokens"`
	ReviewRequired       bool               `json:"review_required"`
	PolicyReason         string             `json:"policy_reason"`
}

var criteria = map[string]string{
	"SUPPORTED":             "The cited evidence and verified analyst findings directly support the claim without a material contradiction.",
	"REFUTED":               "The cited evidence and verified analyst findings directly contradict the claim without a material supporting conflict.",
	"INSUFFICIENT_EVIDENCE": "The available evidence is indirect, irrelevant, incomplete or unable to establish the claim either way.",
	"REQUIRES_REVIEW":       "There are material conflicting findings or a difficult judgment needing a qualified human reviewer.",
}

func validProbability(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }

func (c *JevClient) Decide(ctx context.Context, state any) (Decision, error) {
	if err := c.Validate(); err != nil {
		return Decision{}, err
	}
	model := c.Model
	payload := jevRequest{Model: model, State: state, Questions: map[string]jevQuestion{
		"verdict":          {Type: "choice", Instructions: "Given `claim` and `findings` in the state, which single provisional evidence-based classification best applies? Consider only the cited texts and bounded analyst findings; do not assume any external scientific knowledge. Use INSUFFICIENT_EVIDENCE when direct support is missing.", Criteria: criteria},
		"needs_review":     {Type: "noul", Instructions: "Do the cited findings exhibit material conflict, unresolved ambiguity or other reasons that a qualified human needs to examine them?"},
		"evidence_quality": {Type: "score", Instructions: "How direct and complete are the cited findings for addressing the exact claim?", Criteria: []string{"No pertinent evidence", "Mostly indirect or incomplete", "Some direct evidence with material gaps", "Direct, relevant and reasonably complete evidence"}},
	}}
	body, err := json.Marshal(payload)
	if err != nil {
		return Decision{}, err
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	data, err := httpjson.PostJSON(ctx, c.HTTP, endpoint, c.APIKey, body, 256<<10)
	if err != nil {
		return Decision{}, fmt.Errorf("JEV request: %w", err)
	}
	var parsed jevResponse
	if json.Unmarshal(data, &parsed) != nil || parsed.Model == "" {
		return Decision{}, errors.New("invalid JEV response")
	}
	v, vok := parsed.Answers["verdict"]
	r, rok := parsed.Answers["needs_review"]
	q, qok := parsed.Answers["evidence_quality"]
	if !vok || !rok || !qok || v.Type != "choice" || r.Type != "noul" || q.Type != "score" || v.Confidence == nil || r.Noul == nil || q.Score == nil || !validProbability(*v.Confidence) || !validProbability(*r.Noul) || *q.Score < 0 || *q.Score > 3 {
		return Decision{}, errors.New("invalid JEV answer shape")
	}
	// Official ScoreAnswer requires calibrated level probabilities and a legend.
	// Validate the score as the probability-weighted expectation of 0..3.
	if q.Confidence == nil || !validProbability(*q.Confidence) || len(q.Legend) != 4 || len(q.Probabilities) != 4 {
		return Decision{}, errors.New("JEV evidence score missing confidence, legend or distribution")
	}
	scoreSum, weighted := 0.0, 0.0
	for i := 0; i < 4; i++ {
		key := fmt.Sprint(i)
		probability, ok := q.Probabilities[key]
		if !ok || !validProbability(probability) || q.Legend[key] == "" {
			return Decision{}, errors.New("JEV evidence score contains invalid level")
		}
		scoreSum += probability
		weighted += float64(i) * probability
	}
	if math.Abs(scoreSum-1) > 0.015 || math.Abs(weighted-*q.Score) > 0.04 {
		return Decision{}, errors.New("JEV evidence score distribution inconsistent with score")
	}
	if _, ok := criteria[v.Choice]; !ok {
		return Decision{}, errors.New("JEV chose an unknown verdict")
	}
	if len(v.Probabilities) != len(criteria) {
		return Decision{}, errors.New("JEV missing verdict probabilities")
	}
	sum := 0.0
	for k, p := range v.Probabilities {
		if _, ok := criteria[k]; !ok || !validProbability(p) {
			return Decision{}, errors.New("JEV provided invalid probability")
		}
		sum += p
	}
	if math.Abs(sum-1) > 0.015 {
		return Decision{}, errors.New("JEV probabilities do not sum to 1")
	}
	if v.Probabilities[v.Choice] < 0.001 {
		return Decision{}, errors.New("JEV choice inconsistent with distribution")
	}
	for _, p := range v.Probabilities {
		if p > v.Probabilities[v.Choice]+0.001 {
			return Decision{}, errors.New("JEV choice is not highest probability")
		}
	}
	decision := Decision{Verdict: v.Choice, JEVChoice: v.Choice, Confidence: *v.Confidence, Probabilities: v.Probabilities, ReviewProbability: *r.Noul, EvidenceQualityScore: *q.Score, Model: parsed.Model, InputTokens: parsed.Usage.InputTokens, OutputTokens: parsed.Usage.OutputTokens, ReviewRequired: true, PolicyReason: "Experimental uncalibrated decision; qualified human review required."}
	if decision.Confidence < ReviewThreshold || decision.ReviewProbability >= NoulReviewThreshold {
		decision.Verdict = "REQUIRES_REVIEW"
		decision.PolicyReason = "Low confidence or JEV review flag; gated for human review."
	}
	return decision, nil
}
