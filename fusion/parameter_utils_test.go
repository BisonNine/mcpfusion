/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package fusion

import (
	"testing"
)

func TestSanitizeParameterName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dollar prefix",
			input:    "$select",
			expected: "select",
		},
		{
			name:     "multiple special chars",
			input:    "$@filter#",
			expected: "filter",
		},
		{
			name:     "valid name unchanged",
			input:    "validName123",
			expected: "validName123",
		},
		{
			name:     "underscores and dots preserved",
			input:    "my_param.name",
			expected: "my_param.name",
		},
		{
			name:     "hyphens preserved",
			input:    "param-name",
			expected: "param-name",
		},
		{
			name:     "all special chars",
			input:    "$@#%^&*()",
			expected: "param", // fallback when everything is removed
		},
		{
			name:     "very long name",
			input:    "this_is_a_very_long_parameter_name_that_exceeds_the_64_character_limit_for_mcp",
			expected: "this_is_a_very_long_parameter_name_that_exceeds_the_64_character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeParameterName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeParameterName(%s) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMCPParameterName(t *testing.T) {
	tests := []struct {
		name     string
		param    ParameterConfig
		expected string
	}{
		{
			name: "uses alias when provided",
			param: ParameterConfig{
				Name:  "$select",
				Alias: "select",
			},
			expected: "select",
		},
		{
			name: "sanitizes when no alias",
			param: ParameterConfig{
				Name: "$filter",
			},
			expected: "filter",
		},
		{
			name: "preserves valid name when no alias",
			param: ParameterConfig{
				Name: "validParam",
			},
			expected: "validParam",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MCPParameterName(&tt.param)
			if result != tt.expected {
				t.Errorf("MCPParameterName(%+v) = %s; want %s", tt.param, result, tt.expected)
			}
		})
	}
}

func TestIsValidMCPParameterName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid alphanumeric", "param123", true},
		{"valid with underscore", "my_param", true},
		{"valid with dot", "my.param", true},
		{"valid with hyphen", "my-param", true},
		{"invalid with dollar", "$param", false},
		{"invalid with space", "my param", false},
		{"invalid with special char", "param@123", false},
		{"empty string", "", false},
		{"too long", "this_is_a_very_long_parameter_name_that_exceeds_the_64_character_limit_for_mcp_parameters", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidMCPParameterName(tt.input)
			if result != tt.expected {
				t.Errorf("IsValidMCPParameterName(%s) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParameterNameMapper(t *testing.T) {
	mapper := NewParameterNameMapper()

	// Test adding mappings
	err := mapper.AddMapping("select", "$select")
	if err != nil {
		t.Errorf("AddMapping failed: %v", err)
	}

	err = mapper.AddMapping("filter", "$filter")
	if err != nil {
		t.Errorf("AddMapping failed: %v", err)
	}

	// Test conflict detection
	err = mapper.AddMapping("select", "$another")
	if err == nil {
		t.Error("Expected error for conflicting mapping, got nil")
	}

	// Test retrieving original names
	if orig := mapper.OriginalName("select"); orig != "$select" {
		t.Errorf("OriginalName(select) = %s; want $select", orig)
	}

	if orig := mapper.OriginalName("unknown"); orig != "unknown" {
		t.Errorf("OriginalName(unknown) = %s; want unknown", orig)
	}

	// Test retrieving MCP names
	if mcp := mapper.MCPName("$select"); mcp != "select" {
		t.Errorf("MCPName($select) = %s; want select", mcp)
	}

	if mcp := mapper.MCPName("$unknown"); mcp != "unknown" {
		t.Errorf("MCPName($unknown) = %s; want unknown", mcp)
	}

	// Test mapping args
	args := map[string]any{
		"select": "field1,field2",
		"filter": "isRead eq false",
		"top":    10,
	}

	mapped := mapper.MapArgsToOriginal(args)

	if mapped["$select"] != "field1,field2" {
		t.Errorf("Mapped $select = %v; want field1,field2", mapped["$select"])
	}

	if mapped["$filter"] != "isRead eq false" {
		t.Errorf("Mapped $filter = %v; want isRead eq false", mapped["$filter"])
	}

	if mapped["top"] != 10 {
		t.Errorf("Mapped top = %v; want 10", mapped["top"])
	}
}

func TestValidateParameterNames(t *testing.T) {
	tests := []struct {
		name        string
		params      []ParameterConfig
		expectError bool
	}{
		{
			name: "no conflicts with aliases",
			params: []ParameterConfig{
				{Name: "$select", Alias: "select"},
				{Name: "$filter", Alias: "filter"},
				{Name: "top", Alias: ""},
			},
			expectError: false,
		},
		{
			name: "conflict when sanitized names collide",
			params: []ParameterConfig{
				{Name: "$select"},
				{Name: "select"}, // Both become "select"
			},
			expectError: true,
		},
		{
			name: "no conflict with different aliases",
			params: []ParameterConfig{
				{Name: "$select", Alias: "selectFields"},
				{Name: "select", Alias: "selection"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParameterNames(tt.params)
			if (err != nil) != tt.expectError {
				t.Errorf("ValidateParameterNames() error = %v, expectError = %v", err, tt.expectError)
			}
		})
	}
}

func TestSuggestAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"$select", "select"},
		{"$filter", "filter"},
		{"$orderby", "orderby"},
		{"$top", "top"},
		{"$skip", "skip"},
		{"$expand", "expand"},
		{"$search", "search"},
		{"$count", "count"},
		{"$format", "format"},
		{"$unknown", "unknown"},
		{"@special#param", "specialparam"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SuggestAlias(tt.input)
			if result != tt.expected {
				t.Errorf("SuggestAlias(%s) = %s; want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLookupArg(t *testing.T) {
	args := map[string]any{
		"empty":   "",
		"nilVal":  nil,
		"nonEmp":  "value",
		"zero":    0,
		"falseB":  false,
		"emptySl": []any{},
	}

	if _, ok := lookupArg(args, "missing"); ok {
		t.Error("expected missing key to return false")
	}
	if _, ok := lookupArg(args, "empty"); ok {
		t.Error("expected empty string to return false")
	}
	if _, ok := lookupArg(args, "nilVal"); ok {
		t.Error("expected nil value to return false")
	}
	if v, ok := lookupArg(args, "nonEmp"); !ok || v != "value" {
		t.Errorf("expected nonEmp='value', got %v, %v", v, ok)
	}
	if v, ok := lookupArg(args, "zero"); !ok || v != 0 {
		t.Errorf("expected zero=0, got %v, %v", v, ok)
	}
	if v, ok := lookupArg(args, "falseB"); !ok || v != false {
		t.Errorf("expected falseB=false, got %v, %v", v, ok)
	}
	if _, ok := lookupArg(args, "emptySl"); !ok {
		t.Error("expected empty slice to return true")
	}
}

func TestStripEmptyArgs(t *testing.T) {
	args := map[string]any{
		"empty":  "",
		"nilVal": nil,
		"keep":   "hello",
		"zero":   0,
		"falseB": false,
	}

	stripEmptyArgs(args)

	if _, exists := args["empty"]; exists {
		t.Error("expected 'empty' key to be deleted")
	}
	if _, exists := args["nilVal"]; exists {
		t.Error("expected 'nilVal' key to be deleted")
	}
	if args["keep"] != "hello" {
		t.Errorf("expected 'keep' to remain 'hello', got %v", args["keep"])
	}
	if args["zero"] != 0 {
		t.Errorf("expected 'zero' to remain 0, got %v", args["zero"])
	}
	if args["falseB"] != false {
		t.Errorf("expected 'falseB' to remain false, got %v", args["falseB"])
	}
}
