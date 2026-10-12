package httpjson

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRetryTemporaryFailures(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("request headers wrong")
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	data, err := PostJSON(context.Background(), server.Client(), server.URL, "secret", []byte(`{}`), 100)
	if err != nil || !strings.Contains(string(data), `"ok":true`) || requests.Load() != 2 {
		t.Fatalf("data %s err %v attempts %d", data, err, requests.Load())
	}
}
func TestRejectHugeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 200))) }))
	defer server.Close()
	if _, err := PostJSON(context.Background(), server.Client(), server.URL, "", []byte(`{}`), 64); err == nil {
		t.Fatal("oversized payload accepted")
	}
}
