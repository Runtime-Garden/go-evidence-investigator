package main

import (
	"context"
	"encoding/json"
	"example.com/evidence-investigator/internal/benchmark"
	"example.com/evidence-investigator/internal/casebook"
	"example.com/evidence-investigator/internal/telemetry"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"example.com/evidence-investigator/internal/assessment"
	"example.com/evidence-investigator/internal/decisionflow"
	"example.com/evidence-investigator/internal/investigator"
	"example.com/evidence-investigator/internal/multiagent"
	"example.com/evidence-investigator/internal/server"
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	mode := flag.String("mode", "serve", "serve | fetch | eval | benchmark | healthcheck")
	directory := flag.String("data", getenv("DATA_DIR", "data/demo"), "directory containing corpus.jsonl and claims_dev.jsonl")
	k := flag.Int("k", 5, "BM25 results per claim for eval")
	limit := flag.Int("limit", 0, "eval claims limit (0=all); online benchmark 1..50")
	benchmarkOutput := flag.String("benchmark-output", "benchmark-details.jsonl", "benchmark case JSONL result path")
	flag.Parse()
	if *mode == "fetch" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if err := investigator.FetchSciFact(ctx, *directory); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "SciFact corpus and claims_dev downloaded to", *directory)
		return
	}
	if *mode == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	if *mode != "serve" && *mode != "eval" && *mode != "benchmark" {
		fmt.Fprintln(os.Stderr, "invalid -mode")
		os.Exit(2)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dataDir := *directory
	dataset := "scifact"
	if filepath.Base(dataDir) == "demo" {
		dataset = "synthetic-demo"
	}
	claimsPath := filepath.Join(dataDir, "claims_dev.jsonl")
	if _, err := os.Stat(claimsPath); os.IsNotExist(err) {
		claimsPath = ""
	}
	engine, err := investigator.Load(filepath.Join(dataDir, "corpus.jsonl"), claimsPath, dataset)
	if err != nil {
		logger.Error("dataset loading failed", "error", err)
		os.Exit(1)
	}
	if *mode == "eval" {
		m := engine.Evaluate(*k, *limit)
		_ = json.NewEncoder(os.Stdout).Encode(m)
		return
	}
	var model *assessment.Client
	if getenv("LLM_MODEL", "") != "" {
		model = &assessment.Client{APIKey: os.Getenv("LLM_API_KEY"), Model: os.Getenv("LLM_MODEL"), BaseURL: getenv("LLM_BASE_URL", "https://api.openai.com/v1"), ResponseFormat: getenv("LLM_RESPONSE_FORMAT", "json_schema")}
		if err := model.Validate(); err != nil {
			logger.Error("LLM configuration failed", "error", err)
			os.Exit(1)
		}
	}
	var flow *decisionflow.Flow
	if os.Getenv("JEV_API_KEY") != "" {
		if model == nil {
			logger.Error("JEV requires the LLM analyst and narrator to be configured")
			os.Exit(1)
		}
		jev := &decisionflow.JevClient{APIKey: os.Getenv("JEV_API_KEY"), Model: getenv("JEV_MODEL", "jev-latest"), Endpoint: os.Getenv("JEV_ENDPOINT")}
		if err := jev.Validate(); err != nil {
			logger.Error("JEV configuration failed", "error", err)
			os.Exit(1)
		}
		flow = &decisionflow.Flow{LLM: model, JEV: jev, Stages: telemetry.New()}
	}
	if *mode == "benchmark" {
		if flow == nil {
			logger.Error("benchmark requires LLM and JEV credentials")
			os.Exit(2)
		}
		f, err := os.OpenFile(*benchmarkOutput, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			logger.Error("benchmark output", "error", err)
			os.Exit(1)
		}
		summary, runErr := benchmark.Run(context.Background(), engine, model, flow, *k, *limit, f)
		closeErr := f.Close()
		if runErr != nil || closeErr != nil {
			logger.Error("benchmark failed", "error", runErr, "close_error", closeErr)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(summary)
		return
	}
	var agents *multiagent.Runtime
	if flow != nil {
		agents = &multiagent.Runtime{Engine: engine, Model: model, Flow: flow, Metrics: telemetry.New(), Limits: multiagent.DefaultLimits()}
	}
	addr := getenv("LISTEN_ADDR", "127.0.0.1:8080")
	token := os.Getenv("APP_AUTH_TOKEN")
	reviewToken := os.Getenv("APP_REVIEW_TOKEN")
	if reviewToken == token && token != "" {
		logger.Error("APP_REVIEW_TOKEN must differ from APP_AUTH_TOKEN")
		os.Exit(2)
	}
	if token == "" && !(strings.HasPrefix(addr, "127.0.0.1:") || strings.HasPrefix(addr, "localhost:")) {
		logger.Error("APP_AUTH_TOKEN required when listening on non-loopback address")
		os.Exit(2)
	}
	if !(strings.HasPrefix(addr, "127.0.0.1:") || strings.HasPrefix(addr, "localhost:")) && (len(token) < 24 || len(reviewToken) < 24 || os.Getenv("APP_REVIEWER_ID") == "") {
		logger.Error("non-loopback deployment requires 24+ char auth/review tokens and APP_REVIEWER_ID")
		os.Exit(2)
	}
	var store *casebook.Store
	store, err = casebook.New(getenv("CASE_DIR", "state"))
	if err != nil {
		logger.Error("case storage error", "error", err)
		os.Exit(1)
	}
	httpServer := &http.Server{Addr: addr, Handler: (&server.Server{Engine: engine, Model: model, Flow: flow, Agents: agents, Logger: logger, Store: store, AuthToken: token, ReviewerToken: reviewToken, ReviewerID: os.Getenv("APP_REVIEWER_ID"), Metrics: telemetry.New()}).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 12 * time.Second, WriteTimeout: 190 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-stopped
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()
	logger.Info("starting", "addr", addr, "dataset", dataset, "documents", engine.Documents, "claims", len(engine.Claims), "llm", model != nil, "jev", flow != nil, "multiagent", agents != nil, "auth_enabled", token != "", "case_dir", store.Dir)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
