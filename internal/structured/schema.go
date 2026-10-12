// Package structured implements the deliberately small, strict JSON contract
// needed by Evidence Investigator. It is NOT a general JSON Schema engine.
// Provider-facing schemas and local validation share the same supported subset:
// object, array, string, boolean, integer, number, required fields, enums and
// additionalProperties=false. The implementation uses only Go's standard library.
package structured

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strconv"
	"strings"
)

const (
	maxSchemaDepth = 8
	maxJSONDepth   = 64
	maxJSONBytes   = 1 << 20
)

// SchemaFor derives an OpenAI-compatible strict JSON Schema subset from a
// tagged Go struct. All exported fields must be explicitly named and required;
// enum:"A,B" is supported only on string fields. A fresh schema is returned so
// callers cannot corrupt a cached contract.
func SchemaFor(value any) (map[string]any, error) {
	t := reflect.TypeOf(value)
	if t == nil {
		return nil, errors.New("nil structured output type")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, errors.New("structured output must be a Go struct")
	}
	return specFor(t, 0, map[reflect.Type]bool{})
}

func specFor(t reflect.Type, depth int, ancestors map[reflect.Type]bool) (map[string]any, error) {
	if depth > maxSchemaDepth {
		return nil, errors.New("structured schema too deeply nested")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		if ancestors[t] {
			return nil, fmt.Errorf("recursive schema type %s", t)
		}
		ancestors[t] = true
		defer delete(ancestors, t)
		props := make(map[string]any)
		required := make([]string, 0, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := strings.Split(field.Tag.Get("json"), ",")
			name := tag[0]
			if name == "" || name == "-" || strings.Contains(tagOptions(tag[1:]), ",omitempty,") {
				return nil, fmt.Errorf("%s must have a required JSON name without omitempty", field.Name)
			}
			if _, ok := props[name]; ok {
				return nil, fmt.Errorf("duplicate JSON name %q", name)
			}
			child, err := specFor(field.Type, depth+1, ancestors)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			if enum := field.Tag.Get("enum"); enum != "" {
				if field.Type.Kind() != reflect.String {
					return nil, fmt.Errorf("%s enum must be a string", name)
				}
				values := strings.Split(enum, ",")
				for _, v := range values {
					if v == "" {
						return nil, fmt.Errorf("%s contains empty enum value", name)
					}
				}
				child["enum"] = values
			}
			props[name] = child
			required = append(required, name)
		}
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}, nil
	case reflect.Slice:
		item, err := specFor(t.Elem(), depth+1, ancestors)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": item}, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	default:
		return nil, fmt.Errorf("unsupported schema type %s", t)
	}
}

func tagOptions(options []string) string { return "," + strings.Join(options, ",") + "," }

// Decode rejects duplicate keys, trailing data, missing/unknown fields, nulls,
// wrong types and invalid enum values, then performs an atomic typed decode.
// The destination is only updated after every check succeeds.
func Decode(content string, target any) error {
	if target == nil || reflect.ValueOf(target).Kind() != reflect.Pointer || reflect.ValueOf(target).IsNil() {
		return errors.New("structured target must be a non-nil pointer")
	}
	schema, err := SchemaFor(target)
	if err != nil {
		return err
	}
	if len(content) > maxJSONBytes {
		return fmt.Errorf("JSON exceeds %d-byte limit", maxJSONBytes)
	}
	data := []byte(content)
	if err := rejectDuplicates(data); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // Integer checks must not silently round via float64.
	var node any
	if err := decoder.Decode(&node); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON")
	}
	if err := validate(schema, node, "$", 0); err != nil {
		return fmt.Errorf("output violates JSON Schema: %w", err)
	}
	// Decode into a fresh destination. Never leak partial results on error.
	tmp := reflect.New(reflect.TypeOf(target).Elem())
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.DisallowUnknownFields()
	if err := strict.Decode(tmp.Interface()); err != nil {
		return fmt.Errorf("typed output decode: %w", err)
	}
	reflect.ValueOf(target).Elem().Set(tmp.Elem())
	return nil
}

// validate only accepts schemas created by specFor. It is deliberately not a
// third-party schema dialect interpreter; new keywords require explicit tests.
func validate(schema map[string]any, value any, path string, depth int) error {
	if depth > maxSchemaDepth {
		return fmt.Errorf("%s: nested data exceeds schema depth", path)
	}
	typeName := schema["type"].(string)
	switch typeName {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object", path)
		}
		props := schema["properties"].(map[string]any)
		for _, key := range schema["required"].([]string) {
			if _, present := obj[key]; !present {
				return fmt.Errorf("%s: missing required property %q", path, key)
			}
		}
		for key, field := range obj {
			child, exists := props[key]
			if !exists {
				return fmt.Errorf("%s: unexpected property %q", path, key)
			}
			if err := validate(child.(map[string]any), field, path+"."+key, depth+1); err != nil {
				return err
			}
		}
	case "array":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array", path)
		}
		child := schema["items"].(map[string]any)
		for i, item := range items {
			if err := validate(child, item, fmt.Sprintf("%s[%d]", path, i), depth+1); err != nil {
				return err
			}
		}
	case "string":
		str, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: expected string", path)
		}
		if allowed, ok := schema["enum"].([]string); ok {
			for _, option := range allowed {
				if str == option {
					return nil
				}
			}
			return fmt.Errorf("%s: string %q is not in enum", path, str)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean", path)
		}
	case "integer":
		n, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%s: expected integer", path)
		}
		// Bound work before building big integers from untrusted exponents.
		if len(n.String()) > 256 {
			return fmt.Errorf("%s: integer literal too large", path)
		}
		if idx := strings.IndexAny(n.String(), "eE"); idx >= 0 {
			exponent, err := strconv.ParseInt(n.String()[idx+1:], 10, 32)
			if err != nil || exponent > 1000 || exponent < -1000 {
				return fmt.Errorf("%s: integer exponent out of range", path)
			}
		}
		// big.Rat handles decimals and exponents exactly, including >2^53.
		r, valid := new(big.Rat).SetString(n.String())
		if !valid || !r.IsInt() {
			return fmt.Errorf("%s: expected integer", path)
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			return fmt.Errorf("%s: expected number", path)
		}
	default:
		return fmt.Errorf("%s: unsupported local schema type %q", path, typeName)
	}
	return nil
}

func rejectDuplicates(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := walk(dec, 0); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON token")
	}
	return nil
}

func walk(dec *json.Decoder, depth int) error {
	if depth > maxJSONDepth {
		return errors.New("JSON nested too deeply")
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	open, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch open {
	case '{':
		seen := make(map[string]struct{})
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return errors.New("JSON object key is not string")
			}
			if _, dup := seen[key]; dup {
				return fmt.Errorf("duplicate JSON property %q", key)
			}
			seen[key] = struct{}{}
			if err := walk(dec, depth+1); err != nil {
				return err
			}
		}
		closeTok, err := dec.Token()
		if err != nil || closeTok != json.Delim('}') {
			return errors.New("unclosed JSON object")
		}
	case '[':
		for dec.More() {
			if err := walk(dec, depth+1); err != nil {
				return err
			}
		}
		closeTok, err := dec.Token()
		if err != nil || closeTok != json.Delim(']') {
			return errors.New("unclosed JSON array")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}
