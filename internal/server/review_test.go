package server

import (
	"bytes"
	"encoding/json"
	"example.com/evidence-investigator/internal/casebook"
	"example.com/evidence-investigator/internal/investigator"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthenticationAndReview(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "corpus.jsonl")
	if err := os.WriteFile(corpus, []byte(`{"doc_id":31,"title":"Plants","abstract":["Leaves absorb light in the garden."]}`), 0600); err != nil {
		t.Fatal(err)
	}
	engine, err := investigator.Load(corpus, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := casebook.New(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Engine: engine, Store: ledger, AuthToken: "secret-reader", ReviewerToken: "separate-review", ReviewerID: "reviewer-1"}).Handler()
	call := func(method, path, body, token, reviewKey string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if reviewKey != "" {
			r.Header.Set("X-Review-Token", reviewKey)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/info", "", "", ""); w.Code != 401 {
		t.Fatalf("expected 401, %d", w.Code)
	}
	if w := call("GET", "/api/info", "", "secret-reader", ""); w.Code != 200 {
		t.Fatalf("expected 200, %d", w.Code)
	}
	w := call("POST", "/api/investigate", `{"claim":"Leaves absorb light in the garden"}`, "secret-reader", "")
	if w.Code != 200 {
		t.Fatalf("investigate %d %s", w.Code, w.Body.String())
	}
	var report Investigation
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.CaseID == "" || report.CaseVersion != 1 {
		t.Fatal("case creation missing")
	}
	uri := "/api/cases/" + report.CaseID + "/review"
	payload := `{"version":1,"status":"approved","actor":"forged-name","note":"I read original source"}`
	if w := call("POST", uri, payload, "secret-reader", ""); w.Code != 403 {
		t.Fatalf("missing reviewer token accepted: %d", w.Code)
	}
	if w := call("POST", uri, payload, "secret-reader", "separate-review"); w.Code != 200 {
		t.Fatalf("review failed: %d %s", w.Code, w.Body.String())
	}
	c, err := ledger.Get(report.CaseID)
	if err != nil || c.Events[1].Actor != "reviewer-1" {
		t.Fatalf("review identity wrong %+v %v", c, err)
	}
	if w := call("POST", uri, payload, "secret-reader", "separate-review"); w.Code != 409 {
		t.Fatalf("expected revision conflict, %d", w.Code)
	}
}
