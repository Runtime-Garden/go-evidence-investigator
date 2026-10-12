package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"example.com/evidence-investigator/internal/casebook"
	"example.com/evidence-investigator/internal/telemetry"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/evidence-investigator/internal/assessment"
	"example.com/evidence-investigator/internal/decisionflow"
	"example.com/evidence-investigator/internal/investigator"
	"example.com/evidence-investigator/internal/multiagent"
)

//go:embed web/*
var webAssets embed.FS

type Server struct {
	Engine        *investigator.Engine
	Model         *assessment.Client
	Flow          *decisionflow.Flow
	Agents        *multiagent.Runtime
	Logger        *slog.Logger
	Store         *casebook.Store
	AuthToken     string
	ReviewerToken string
	ReviewerID    string
	Metrics       *telemetry.Metrics
}

type Investigation struct {
	CaseID         string                 `json:"case_id,omitempty"`
	CaseVersion    int                    `json:"case_version,omitempty"`
	Claim          string                 `json:"claim"`
	Dataset        string                 `json:"dataset"`
	Verdict        string                 `json:"verdict"`
	Evidence       []investigator.Passage `json:"evidence"`
	Assessment     *assessment.Assessment `json:"assessment,omitempty"`
	JEVReport      *decisionflow.Output   `json:"jev_report,omitempty"`
	AgentTrace     *multiagent.Trace      `json:"agent_trace,omitempty"`
	Notice         string                 `json:"notice"`
	ReviewRequired bool                   `json:"review_required"`
	TookMS         int64                  `json:"took_ms"`
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func errResponse(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webAssets, "web")
	assets := http.FileServer(http.FS(sub))
	mux.Handle("GET /", assets)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, 200, map[string]any{"status": "ok", "dataset": s.Engine.Dataset, "documents": s.Engine.Documents})
	})
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, 200, map[string]any{"dataset": s.Engine.Dataset, "documents": s.Engine.Documents, "claims": len(s.Engine.Claims), "llm_available": s.Model != nil && s.Model.Ready(), "jev_available": s.Flow != nil && s.Flow.JEV != nil && s.Flow.JEV.Ready() && s.Model != nil && s.Model.Ready(), "claim_verdict_is_authoritative": false, "multiagent_available": s.Agents != nil})
	})
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, 200, s.Metrics.Snapshot()) })
	mux.HandleFunc("GET /api/cases", func(w http.ResponseWriter, r *http.Request) {
		if s.Store == nil {
			errResponse(w, 503, "case storage disabled")
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		cases, err := s.Store.List(limit)
		if err != nil {
			errResponse(w, 500, "case storage error")
			return
		}
		// list includes full reports. Endpoint is protected by same bearer key.
		jsonResponse(w, 200, cases)
	})
	mux.HandleFunc("GET /api/cases/{id}", func(w http.ResponseWriter, r *http.Request) {
		if s.Store == nil {
			errResponse(w, 503, "case storage disabled")
			return
		}
		c, err := s.Store.Get(r.PathValue("id"))
		if err != nil {
			errResponse(w, 404, "case not found or invalid")
			return
		}
		jsonResponse(w, 200, c)
	})
	mux.HandleFunc("POST /api/cases/{id}/review", func(w http.ResponseWriter, r *http.Request) {
		if s.Store == nil {
			errResponse(w, 503, "case storage disabled")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var v struct {
			Version int    `json:"version"`
			Status  string `json:"status"`
			Actor   string `json:"actor"`
			Note    string `json:"note"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&v); err != nil {
			errResponse(w, 400, "invalid review payload")
			return
		}
		if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
			errResponse(w, 400, "trailing review JSON")
			return
		}
		actor := v.Actor
		if s.ReviewerID != "" {
			actor = s.ReviewerID
		}
		c, err := s.Store.Review(r.PathValue("id"), v.Version, v.Status, actor, v.Note)
		if errors.Is(err, casebook.ErrVersionConflict) {
			errResponse(w, 409, "review version conflict")
			return
		}
		if errors.Is(err, casebook.ErrNotFound) {
			errResponse(w, 404, "case not found")
			return
		}
		if err != nil {
			errResponse(w, 400, "review rejected")
			return
		}
		jsonResponse(w, 200, c)
	})
	mux.HandleFunc("GET /api/claims", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		jsonResponse(w, 200, investigator.ClaimsPreview(s.Engine.Claims, limit))
	})
	mux.HandleFunc("POST /api/investigate", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		var req struct {
			Claim     string `json:"claim"`
			TopK      int    `json:"top_k"`
			UseModel  bool   `json:"use_model"`
			UseJEV    bool   `json:"use_jev"`
			UseAgents bool   `json:"use_agents"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			errResponse(w, 400, "invalid JSON request")
			return
		}
		if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
			errResponse(w, 400, "unexpected data after JSON object")
			return
		}
		req.Claim = strings.TrimSpace(req.Claim)
		if len([]rune(req.Claim)) < 5 || len([]rune(req.Claim)) > 1000 {
			errResponse(w, 400, "claim length must be 5 to 1000 characters")
			return
		}
		if req.TopK == 0 {
			req.TopK = 5
		}
		if req.TopK < 1 || req.TopK > 10 {
			errResponse(w, 400, "top_k must be 1 to 10")
			return
		}
		if (req.UseJEV && req.UseModel) || (req.UseAgents && (req.UseJEV || req.UseModel)) {
			errResponse(w, 400, "choose either legacy model assessment or JEV workflow")
			return
		}
		if req.UseAgents && s.Agents == nil {
			errResponse(w, 400, "autonomous multi-agent workflow is not configured")
			return
		}
		if req.UseJEV && (s.Flow == nil || s.Flow.JEV == nil || !s.Flow.JEV.Ready() || s.Model == nil || !s.Model.Ready()) {
			errResponse(w, 400, "JEV workflow is not configured")
			return
		}
		if req.UseModel && (s.Model == nil || !s.Model.Ready()) {
			errResponse(w, 400, "model assessment is not configured")
			return
		}
		started := time.Now()
		evidence := s.Engine.Search(req.Claim, req.TopK)
		if evidence == nil {
			evidence = []investigator.Passage{}
		}
		for _, p := range evidence {
			if !investigator.AuditQuote(p) {
				errResponse(w, 500, "corpus integrity check failed")
				return
			}
		}
		result := Investigation{Claim: req.Claim, Dataset: s.Engine.Dataset, Verdict: "NOT_ASSESSED", Evidence: evidence, ReviewRequired: true,
			Notice: "Lexical retrieval ranking is NOT proof that the claim is true or false. All results require human review."}
		if len(evidence) == 0 {
			result.Notice = "No matching source passages found; this does not prove a claim false."
		}
		if req.UseAgents {
			ctx, cancel := context.WithTimeout(r.Context(), 175*time.Second)
			defer cancel()
			research, err := s.Agents.Run(ctx, req.Claim, req.TopK)
			if err != nil {
				if s.Logger != nil {
					s.Logger.Warn("multi-agent workflow failed", "error", err)
				}
				errResponse(w, 502, "multi-agent workflow failed or exceeded its budget; no verdict issued")
				return
			}
			result.Evidence = research.Evidence
			result.JEVReport = &research.Report
			result.AgentTrace = &multiagent.Trace{Events: research.Events, ModelCalls: research.ModelCalls, StopReason: research.StopReason, DecisionCalls: research.DecisionCalls}
			result.Verdict = research.Report.Decision.Verdict
			result.Notice = "Autonomous bounded research, JEV-proposed decision, policy validation, and human review required."
		} else if req.UseJEV {
			if len(evidence) == 0 {
				result.Notice = "No matching evidence. JEV was not called and no claim decision was issued."
			} else {
				ctx, cancel := context.WithTimeout(r.Context(), 65*time.Second)
				defer cancel()
				report, err := s.Flow.Run(ctx, req.Claim, evidence)
				if err != nil {
					if s.Logger != nil {
						s.Logger.Warn("JEV pipeline failed", "error", err)
					}
					errResponse(w, 502, "JEV workflow failed validation or provider unavailable; no verdict issued")
					return
				}
				result.JEVReport = &report
				result.Verdict = report.Decision.Verdict
				result.Notice = "JEV-based provisional decision with safety policy; confidence is not proven accuracy. Human review required."
			}
		} else if req.UseModel {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			evaluation, err := s.Model.Assess(ctx, req.Claim, evidence)
			if err != nil {
				if s.Logger != nil {
					s.Logger.Warn("model assessment failed", "error", err)
				}
				errResponse(w, 502, "model assessment unavailable or rejected; retry without model")
				return
			}
			result.Assessment = &evaluation
			result.Verdict = evaluation.Verdict
			result.Notice = "Model-proposed verdict is not authoritative. Exact quote membership checked; semantic correctness is NOT proven. Human review required."
		}
		result.TookMS = time.Since(started).Milliseconds()
		if s.Store != nil {
			verdict := result.Verdict
			c, err := s.Store.Create(result, verdict)
			if err != nil {
				errResponse(w, 500, "unable to persist investigation; result withheld")
				return
			}
			result.CaseID = c.ID
			result.CaseVersion = c.Version
		}
		jsonResponse(w, 200, result)
	})
	// Simple bounded concurrency limits burst cost and memory usage.
	slots := make(chan struct{}, 8)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && s.AuthToken != "" {
			provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			expectedHash := sha256.Sum256([]byte(s.AuthToken))
			receivedHash := sha256.Sum256([]byte(provided))
			if subtle.ConstantTimeCompare(expectedHash[:], receivedHash[:]) != 1 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				errResponse(w, 401, "authentication required")
				return
			}
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/cases/") && strings.HasSuffix(r.URL.Path, "/review") && s.ReviewerToken != "" {
			raw := r.Header.Get("X-Review-Token")
			a := sha256.Sum256([]byte(s.ReviewerToken))
			b := sha256.Sum256([]byte(raw))
			if subtle.ConstantTimeCompare(a[:], b[:]) != 1 {
				errResponse(w, 403, "reviewer authorization required")
				return
			}
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/") {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				errResponse(w, 429, "server busy; retry later")
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		if r.URL.Path != "/healthz" && s.Logger != nil {
			s.Logger.Info("request", "method", r.Method, "path", r.URL.Path)
		} // No query/body or keys in logs.
		started := time.Now()
		mux.ServeHTTP(w, r)
		s.Metrics.Observe("http_"+r.Pattern, time.Since(started), nil)
	})
}
