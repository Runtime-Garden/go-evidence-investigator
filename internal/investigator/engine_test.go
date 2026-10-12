package investigator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchAndAudit(t *testing.T) {
	dir := t.TempDir()
	corpus := filepath.Join(dir, "corpus.jsonl")
	content := `{"doc_id":17,"title":"Tomato growth","abstract":["Cold temperatures reduced tomato yield.","The study was synthetic."]}` + "\n" +
		`{"doc_id":18,"title":"Ocean currents","abstract":["Currents transport seawater."]}` + "\n"
	if err := os.WriteFile(corpus, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	e, err := Load(corpus, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	result := e.Search("cold temperature reduced tomato yield", 3)
	if len(result) == 0 || result[0].DocumentID != 17 {
		t.Fatalf("unexpected retrieval: %+v", result)
	}
	if result[0].SourceID != "test:17:0" {
		t.Fatal("bad passage ID")
	}
	if !AuditQuote(result[0]) {
		t.Fatal("hash invalid")
	}
	result[0].Quote += " altered"
	if AuditQuote(result[0]) {
		t.Fatal("tampered quote accepted")
	}
}

func TestEvaluateOnlyGoldLabeledClaims(t *testing.T) {
	e := &Engine{Dataset: "test", Claims: []Claim{{ID: 1, Claim: "unmatched"}}}
	m := e.Evaluate(5, 0)
	if m.Evaluated != 0 || m.SkippedNoGold != 1 {
		t.Fatalf("unexpected metrics: %+v", m)
	}
}

func TestInvalidFileRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corpus.jsonl")
	if err := os.WriteFile(path, []byte(`{"doc_id":0,"title":"","abstract":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path, "", "x")
	if err == nil || !strings.Contains(err.Error(), "invalid document") {
		t.Fatalf("expected schema rejection, got %v", err)
	}
}

func TestPreviewDoesNotLeakLabels(t *testing.T) {
	c := []Claim{{ID: 5, Claim: "Something", Evidence: map[string][]struct {
		Label     string `json:"label"`
		Sentences []int  `json:"sentences"`
	}{"42": {{Label: "SUPPORT", Sentences: []int{0}}}}}}
	preview := ClaimsPreview(c, 10)
	if len(preview) != 1 || preview[0].Claim != "Something" {
		t.Fatal(preview)
	}
}
