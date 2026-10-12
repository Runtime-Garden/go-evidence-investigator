package structured

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type numberContract struct {
	Count int64   `json:"count"`
	Value float64 `json:"value"`
	Valid bool    `json:"valid"`
}

type subContract struct {
	Name string `json:"name"`
}
type nestedContract struct {
	Items [][]subContract `json:"items"`
}

func TestStandardLibrarySchemaCompatibility(t *testing.T) {
	schema, err := SchemaFor(&numberContract{})
	if err != nil {
		t.Fatal(err)
	}
	buf, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"additionalProperties":false`, `"count"`, `"integer"`, `"required"`, `"number"`} {
		if !strings.Contains(string(buf), fragment) {
			t.Fatalf("schema missing %s: %s", fragment, buf)
		}
	}
	if schema["type"] != "object" {
		t.Fatal("provider schema must remain strict object")
	}
}

func TestStrictPrimitiveAndNestedArrayValidation(t *testing.T) {
	validNumbers := `{"count":2,"value":2.5,"valid":false}`
	var out numberContract
	if err := Decode(validNumbers, &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 2 || out.Value != 2.5 || out.Valid {
		t.Fatalf("unexpected decoded output: %+v", out)
	}
	var nested nestedContract
	if err := Decode(`{"items":[[{"name":"first"}],[]]}`, &nested); err != nil {
		t.Fatal(err)
	}
	if len(nested.Items) != 2 || nested.Items[0][0].Name != "first" {
		t.Fatal(nested)
	}
}

func TestContractRejectsInvalidNumbersAndNulls(t *testing.T) {
	bad := []string{
		`{"count":1.25,"value":1.0,"valid":true}`,
		`{"count":"1","value":1.0,"valid":true}`,
		`{"count":1,"value":"1","valid":true}`,
		`{"count":1,"value":null,"valid":true}`,
		`{"count":1,"value":1,"valid":0}`,
		`{"count":1e999999999,"value":1,"valid":true}`,
		`{"count":9223372036854775808,"value":1,"valid":true}`,
		`{"count":1,"value":1,"valid":true,"extra":0}`,
		`{"count":1,"value":1,"valid":true,"count":3}`,
	}
	for _, body := range bad {
		t.Run(body, func(t *testing.T) {
			var result numberContract
			if err := Decode(body, &result); err == nil {
				t.Fatalf("invalid input accepted: %s", body)
			}
		})
	}
}

func TestDecodeIsAtomic(t *testing.T) {
	before := numberContract{Count: 31, Value: 5.5, Valid: true}
	out := before
	if err := Decode(`{"count":1,"value":1e999,"valid":false}`, &out); err == nil {
		t.Fatal("expected type decode failure on float range")
	}
	if out != before {
		t.Fatalf("partial output leaked on validation error: %#v", out)
	}
}

func TestTooLargeAndRecursiveSchemas(t *testing.T) {
	var out testResult
	if err := Decode(strings.Repeat("x", maxJSONBytes+1), &out); err == nil {
		t.Fatal("large output accepted")
	}
	if err := Decode(`{"findings":`+strings.Repeat("[", maxJSONDepth+1)+`{}`+strings.Repeat("]", maxJSONDepth+1)+`}`, &out); err == nil {
		t.Fatal("deep nesting accepted")
	}
	type cycle struct {
		Next *cycle `json:"next"`
	}
	if _, err := SchemaFor(cycle{}); err == nil {
		t.Fatal("recursive schema accepted")
	}
}

func TestInvalidSchemaDeclarations(t *testing.T) {
	for _, v := range []any{
		struct {
			Value string `json:"value,omitempty"`
		}{},
		struct {
			Value map[string]string `json:"value"`
		}{},
		struct {
			Value int `json:"value" enum:"a,b"`
		}{},
	} {
		if _, err := SchemaFor(v); err == nil {
			t.Fatalf("unsupported type accepted: %T", v)
		}
	}
	// Construct a duplicate-tag declaration dynamically so go vet does not
	// reject the test fixture before our runtime rejection can be exercised.
	double := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(""), Tag: `json:"dup"`},
		{Name: "B", Type: reflect.TypeOf(""), Tag: `json:"dup"`},
	})
	if _, err := SchemaFor(reflect.New(double).Elem().Interface()); err == nil {
		t.Fatal("duplicate JSON field names accepted")
	}
}

func TestCallerCannotMutateAnotherSchema(t *testing.T) {
	a, _ := SchemaFor(testResult{})
	a["type"] = "garbage"
	b, err := SchemaFor(testResult{})
	if err != nil || b["type"] != "object" {
		t.Fatalf("caller mutated source contract: %v %v", b, err)
	}
}
