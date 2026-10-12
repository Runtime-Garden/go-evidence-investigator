package assessment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/evidence-investigator/internal/investigator"
)

func TestRejectInventedCitation(t *testing.T) {
	passages := []investigator.Passage{{SourceID: "demo:7:0", Quote: "Temperature reduced yield."}}
	a := Assessment{Verdict: "SUPPORTED", Explanation: "Based on quote", Citations: []Citation{{SourceID: "demo:7:0", Quote: "unrelated quote"}}}
	if ValidateAssessment(a, passages) == nil {
		t.Fatal("fabricated quote accepted")
	}
	a.Citations[0].Quote = "Temperature reduced yield."
	if err := ValidateAssessment(a, passages); err != nil {
		t.Fatalf("valid quote rejected: %v", err)
	}
	a.Citations = nil
	if ValidateAssessment(a, passages) == nil {
		t.Fatal("unsupported unsupported verdict")
	}
}

func TestEnforceHttpsExceptLoopback(t *testing.T) {
	for _, v := range []string{"http://example.org/v1", "ftp://127.0.0.1/v1", "https://user:pass@host/v1"} {
		c := Client{BaseURL: v, Model: "demo"}
		if c.Validate() == nil {
			t.Fatalf("unsafe address allowed: %s", v)
		}
	}
	if err := (&Client{BaseURL: "http://127.0.0.1:11434/v1", Model: "demo"}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFakeProviderResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"REFUTED\",\"explanation\":\"A source contradicts the claim.\",\"citations\":[{\"source_id\":\"demo:1:0\",\"quote\":\"lower yield\"}]}"}}]}`))
	}))
	defer server.Close()
	c := Client{Model: "test", BaseURL: server.URL, HTTP: server.Client()}
	p := []investigator.Passage{{SourceID: "demo:1:0", Quote: "Cold plants had lower yield."}}
	a, err := c.Assess(context.Background(), "Cold increases yield", p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Verdict != "REFUTED" || !a.ReviewRequired {
		t.Fatal(a)
	}
}
