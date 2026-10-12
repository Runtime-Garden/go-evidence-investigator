package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/evidence-investigator/internal/investigator"
)

func TestEndpoints(t *testing.T) {
	f := filepath.Join(t.TempDir(), "corpus.jsonl")
	if err := os.WriteFile(f, []byte(`{"doc_id":25,"title":"Garden health","abstract":["Green leaves absorb light."]}`), 0600); err != nil {
		t.Fatal(err)
	}
	e, err := investigator.Load(f, "", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Engine: e}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/investigate", bytes.NewBufferString(`{"claim":"Green leaves absorb light","top_k":5}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	var report Investigation
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Verdict != "NOT_ASSESSED" || !report.ReviewRequired || len(report.Evidence) == 0 {
		t.Fatalf("bad report: %+v", report)
	}
	if strings.Contains(w.Body.String(), "\"label\"") {
		t.Fatal("label leaked")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/investigate", bytes.NewBufferString(`{"claim":"too short"} {}`)))
	if w.Code != 400 {
		t.Fatalf("allowed trailing JSON: %d", w.Code)
	}
}
