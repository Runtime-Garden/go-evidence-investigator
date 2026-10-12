package structured

import (
	"strings"
	"testing"
)

type testEvidence struct {
	Source   string `json:"source"`
	Relation string `json:"relation" enum:"SUPPORTS,CONTRADICTS"`
}
type testResult struct {
	Findings []testEvidence `json:"findings"`
}

func TestSchemaAndDecode(t *testing.T) {
	schema, err := SchemaFor(&testResult{})
	if err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatal("unknown fields are allowed")
	}
	props := schema["properties"].(map[string]any)
	items := props["findings"].(map[string]any)["items"].(map[string]any)
	relation := items["properties"].(map[string]any)["relation"].(map[string]any)
	if len(relation["enum"].([]string)) != 2 {
		t.Fatal("enum not reflected")
	}
	var result testResult
	if err := Decode(`{"findings":[{"source":"source:1","relation":"SUPPORTS"}]}`, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Relation != "SUPPORTS" {
		t.Fatal(result)
	}
}
func TestRejectInvalidStructuredOutput(t *testing.T) {
	invalid := []string{
		`{"findings":[{"source":"s","relation":"UNKNOWN"}]}`,
		`{"findings":[{"source":"s"}]}`,
		`{"findings":[{"source":"s","relation":"SUPPORTS","extra":true}]}`,
		`{"findings":[{"source":1,"relation":"SUPPORTS"}]}`,
		`{"findings":[{"source":"s","relation":"SUPPORTS","relation":"CONTRADICTS"}]}`,
		`{"findings":[{"source":"s","relation":"SUPPORTS"}]} trailing`,
		`{"findings":null}`,
		`{"findings":[] ,"extra":"none"}`,
	}
	for _, v := range invalid {
		var out testResult
		if err := Decode(v, &out); err == nil {
			t.Fatalf("invalid output accepted: %s", v)
		}
	}
}
func TestDuplicateNested(t *testing.T) {
	var out testResult
	err := Decode(`{"findings":[{"source":"s","relation":"SUPPORTS","source":"b"}]}`, &out)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate not rejected: %v", err)
	}
}
