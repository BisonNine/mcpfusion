/******************************************************************************
 * Copyright (c) 2025-2026 Tenebris Technologies Inc.                         *
 * Please see LICENSE file for details.                                       *
 ******************************************************************************/

package fusion

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.bison9.com/mcpfusion/global"
)

func Test_setNestedValue(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]any
		key      string
		value    any
		expected map[string]any
	}{
		{
			name:    "single key flat assignment",
			initial: map[string]any{},
			key:     "subject",
			value:   "Meeting",
			expected: map[string]any{
				"subject": "Meeting",
			},
		},
		{
			name:    "two-level nesting",
			initial: map[string]any{},
			key:     "start.dateTime",
			value:   "2025-01-15T10:00:00Z",
			expected: map[string]any{
				"start": map[string]any{
					"dateTime": "2025-01-15T10:00:00Z",
				},
			},
		},
		{
			name:    "three-level nesting",
			initial: map[string]any{},
			key:     "a.b.c",
			value:   42,
			expected: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": 42,
					},
				},
			},
		},
		{
			name: "shared prefix merges into one object",
			initial: func() map[string]any {
				m := map[string]any{}
				setNestedValue(m, "start.dateTime", "2025-01-15T10:00:00Z")
				return m
			}(),
			key:   "start.timeZone",
			value: "America/New_York",
			expected: map[string]any{
				"start": map[string]any{
					"dateTime": "2025-01-15T10:00:00Z",
					"timeZone": "America/New_York",
				},
			},
		},
		{
			name: "overwrite non-map existing key",
			initial: map[string]any{
				"start": "scalar-value",
			},
			key:   "start.dateTime",
			value: "2025-01-15T10:00:00Z",
			expected: map[string]any{
				"start": map[string]any{
					"dateTime": "2025-01-15T10:00:00Z",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setNestedValue(tt.initial, tt.key, tt.value)
			assert.Equal(t, tt.expected, tt.initial)
		})
	}
}

func TestMapper_BuildRequestBody_DotNotation(t *testing.T) {
	mapper := NewMapper()

	params := []ParameterConfig{
		{
			Name:     "startDateTime",
			Type:     ParameterTypeString,
			Required: true,
			Location: ParameterLocationBody,
			Transform: &TransformConfig{
				TargetName: "start.dateTime",
				Expression: ".",
			},
		},
		{
			Name:     "startTimeZone",
			Type:     ParameterTypeString,
			Required: false,
			Location: ParameterLocationBody,
			Transform: &TransformConfig{
				TargetName: "start.timeZone",
				Expression: ".",
			},
		},
		{
			Name:     "subject",
			Type:     ParameterTypeString,
			Required: true,
			Location: ParameterLocationBody,
		},
	}

	args := map[string]any{
		"startDateTime": "2025-07-01T10:00:00Z",
		"startTimeZone": "America/New_York",
		"subject":       "Team Meeting",
	}

	body, err := mapper.BuildRequestBody(params, args, nil)
	require.NoError(t, err)
	require.NotNil(t, body)

	// Verify the flat key
	assert.Equal(t, "Team Meeting", body["subject"])

	// Verify the nested structure
	startObj, ok := body["start"].(map[string]any)
	require.True(t, ok, "start should be a nested map")
	assert.Equal(t, "2025-07-01T10:00:00Z", startObj["dateTime"])
	assert.Equal(t, "America/New_York", startObj["timeZone"])
}

func TestMapper_BuildRequestBody_IdentityPassthrough(t *testing.T) {
	mapper := NewMapper()

	params := []ParameterConfig{
		{
			Name:     "description",
			Type:     ParameterTypeString,
			Required: true,
			Location: ParameterLocationBody,
			Transform: &TransformConfig{
				TargetName: "description",
				Expression: ".",
			},
		},
	}

	args := map[string]any{
		"description": "A simple test value",
	}

	body, err := mapper.BuildRequestBody(params, args, nil)
	require.NoError(t, err)
	require.NotNil(t, body)

	// Identity expression "." should pass the value through unchanged
	assert.Equal(t, "A simple test value", body["description"])
}

func TestMapper_BuildRequestBody_WithEncoding(t *testing.T) {
	mapper := NewMapper()

	params := []ParameterConfig{
		{Name: "to", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
		{Name: "subject", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
		{Name: "body", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
	}
	args := map[string]any{
		"to":      "alice@example.com",
		"subject": "Test",
		"body":    "Hello",
	}
	rbConfig := &RequestBodyConfig{
		Encoding:    "rfc2822_base64url",
		WrapperPath: "message.raw",
	}

	result, err := mapper.BuildRequestBody(params, args, rbConfig)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Flat params should be gone
	assert.Nil(t, result["to"])
	assert.Nil(t, result["subject"])
	assert.Nil(t, result["body"])

	// Encoded value should be at message.raw
	msgObj, ok := result["message"].(map[string]any)
	require.True(t, ok, "message should be a nested map")
	rawStr, ok := msgObj["raw"].(string)
	require.True(t, ok, "raw should be a string")

	decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(rawStr)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "To: alice@example.com")
	assert.Contains(t, string(decoded), "Subject: Test")
	assert.True(t, strings.HasSuffix(string(decoded), "\r\n\r\nHello"))
}

func TestMapper_BuildRequestBody_WithEncoding_MixedParams(t *testing.T) {
	mapper := NewMapper()

	params := []ParameterConfig{
		{
			Name:     "messageId",
			Type:     ParameterTypeString,
			Required: true,
			Location: ParameterLocationBody,
			Transform: &TransformConfig{
				TargetName: "message.threadId",
				Expression: ".",
			},
		},
		{Name: "to", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
		{Name: "subject", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
		{Name: "body", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
	}
	args := map[string]any{
		"messageId": "thread123",
		"to":        "bob@example.com",
		"subject":   "Reply",
		"body":      "Thanks!",
	}
	rbConfig := &RequestBodyConfig{
		Encoding:    "rfc2822_base64url",
		WrapperPath: "message.raw",
	}

	result, err := mapper.BuildRequestBody(params, args, rbConfig)
	require.NoError(t, err)
	require.NotNil(t, result)

	// messageId with targetName should bypass encoding → message.threadId
	msgObj, ok := result["message"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "thread123", msgObj["threadId"])

	// Flat params should be encoded → message.raw
	rawStr, ok := msgObj["raw"].(string)
	require.True(t, ok, "raw should be a string")
	decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(rawStr)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), "To: bob@example.com")
	assert.Contains(t, string(decoded), "Subject: Reply")

	// Flat params should not appear at top level
	assert.Nil(t, result["to"])
	assert.Nil(t, result["subject"])
	assert.Nil(t, result["body"])
}

func TestMapper_BuildRequestBody_NoEncoding_FlatParams(t *testing.T) {
	mapper := NewMapper()

	params := []ParameterConfig{
		{Name: "to", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
		{Name: "subject", Type: ParameterTypeString, Required: true, Location: ParameterLocationBody},
	}
	args := map[string]any{
		"to":      "alice@example.com",
		"subject": "Test",
	}

	// nil requestBody = backwards compatible behavior
	result, err := mapper.BuildRequestBody(params, args, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "alice@example.com", result["to"])
	assert.Equal(t, "Test", result["subject"])
}

func TestMapper_ApplyQueryParams_EmptyOptionalOmitted(t *testing.T) {
	mapper := NewMapper()

	params := []ParameterConfig{
		{
			Name:     "foo",
			Type:     ParameterTypeString,
			Location: ParameterLocationQuery,
			Required: false,
		},
		{
			Name:     "withDefault",
			Type:     ParameterTypeString,
			Location: ParameterLocationQuery,
			Required: false,
			Default:  "defaultVal",
		},
		{
			Name:     "zeroNum",
			Type:     ParameterTypeNumber,
			Location: ParameterLocationQuery,
			Required: false,
		},
		{
			Name:     "falseBool",
			Type:     ParameterTypeBoolean,
			Location: ParameterLocationQuery,
			Required: false,
		},
	}

	tests := []struct {
		name      string
		args      map[string]any
		wantQuery string
	}{
		{
			name:      "optional param omitted from args",
			args:      map[string]any{},
			wantQuery: "withDefault=defaultVal",
		},
		{
			name: "optional param supplied as empty string",
			args: map[string]any{
				"foo":         "",
				"withDefault": "",
			},
			wantQuery: "withDefault=defaultVal",
		},
		{
			name: "optional param supplied as nil",
			args: map[string]any{
				"foo":         nil,
				"withDefault": nil,
			},
			wantQuery: "withDefault=defaultVal",
		},
		{
			name: "zero and false values are preserved",
			args: map[string]any{
				"foo":       "",
				"zeroNum":   0,
				"falseBool": false,
			},
			wantQuery: "falseBool=false&withDefault=defaultVal&zeroNum=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://api.example.com/items", nil)
			require.NoError(t, err)

			err = mapper.ApplyQueryParams(req, params, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.wantQuery, req.URL.RawQuery)
		})
	}
}

func TestMapper_ApplyHeaders_And_BuildRequestBody_EmptyOptionalOmitted(t *testing.T) {
	mapper := NewMapper()

	params := []ParameterConfig{
		{
			Name:     "X-Optional",
			Type:     ParameterTypeString,
			Location: ParameterLocationHeader,
			Required: false,
		},
		{
			Name:     "X-Default",
			Type:     ParameterTypeString,
			Location: ParameterLocationHeader,
			Required: false,
			Default:  "def-hdr",
		},
		{
			Name:     "optBody",
			Type:     ParameterTypeString,
			Location: ParameterLocationBody,
			Required: false,
		},
		{
			Name:     "defBody",
			Type:     ParameterTypeString,
			Location: ParameterLocationBody,
			Required: false,
			Default:  "def-body",
		},
	}

	args := map[string]any{
		"X-Optional": "",
		"X-Default":  "",
		"optBody":    "",
		"defBody":    nil,
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.example.com/items", nil)
	require.NoError(t, err)
	require.NoError(t, mapper.ApplyHeaders(req, params, args))
	assert.Empty(t, req.Header.Get("X-Optional"))
	assert.Equal(t, "def-hdr", req.Header.Get("X-Default"))

	body, err := mapper.BuildRequestBody(params, args, nil)
	require.NoError(t, err)
	_, hasOpt := body["optBody"]
	assert.False(t, hasOpt, "empty optional body parameter should be omitted")
	assert.Equal(t, "def-body", body["defBody"])
}

func TestValidator_EmptyOptionalAndRequiredParameters(t *testing.T) {
	validator := NewValidator()

	params := []ParameterConfig{
		{
			Name:     "reqParam",
			Type:     ParameterTypeString,
			Location: ParameterLocationQuery,
			Required: true,
		},
		{
			Name:     "optPattern",
			Type:     ParameterTypeString,
			Location: ParameterLocationQuery,
			Required: false,
			Validation: &ValidationConfig{
				Pattern: "^\\d{8}$",
			},
		},
		{
			Name:     "optNumber",
			Type:     ParameterTypeNumber,
			Location: ParameterLocationQuery,
			Required: false,
		},
		{
			Name:     "optDefault",
			Type:     ParameterTypeString,
			Location: ParameterLocationQuery,
			Required: false,
			Default:  "fallback",
		},
	}

	// Optional parameters passed as "" or nil should be skipped (or defaulted) without validation errors
	args := map[string]any{
		"reqParam":   "ok",
		"optPattern": "",
		"optNumber":  nil,
		"optDefault": "",
	}
	require.NoError(t, validator.ValidateParameters(params, args))
	_, hasOptPattern := args["optPattern"]
	assert.False(t, hasOptPattern, "empty optional parameter should be removed from args")
	_, hasOptNumber := args["optNumber"]
	assert.False(t, hasOptNumber, "nil optional parameter should be removed from args")
	assert.Equal(t, "fallback", args["optDefault"], "empty optional parameter with default should receive default")

	// Required parameter passed as "" should fail validation
	err := validator.ValidateParameters(params, map[string]any{"reqParam": ""})
	require.Error(t, err, "required parameter with empty string should fail validation")
}

func TestHTTPHandler_EmptyOptionalQueryParamNotSent(t *testing.T) {
	var receivedRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	f, err := New(WithConfig(&Config{
		Services: map[string]*ServiceConfig{
			"test": {
				Name:    "Test",
				BaseURL: srv.URL,
				Auth:    AuthConfig{Type: AuthTypeNone},
				Endpoints: []EndpointConfig{
					{
						ID:          "list",
						Name:        "List",
						Description: "list items",
						Method:      "GET",
						Path:        "/items",
						Parameters: []ParameterConfig{
							{
								Name:     "foo",
								Type:     ParameterTypeString,
								Location: ParameterLocationQuery,
								Required: false,
							},
							{
								Name:     "$orderby",
								Alias:    "orderby",
								Type:     ParameterTypeString,
								Location: ParameterLocationQuery,
								Required: false,
							},
						},
						Response: ResponseConfig{Type: ResponseTypeText},
					},
				},
			},
		},
	}))
	require.NoError(t, err)

	svc := f.Service("test")
	ep := svc.EndpointByID("list")
	handler := NewHTTPHandler(f, svc, ep)

	ctx := context.WithValue(context.Background(), global.TenantContextKey, &TenantContext{
		TenantHash:  "tenant-test",
		ServiceName: "test",
	})

	_, err = handler.Handle(ctx, map[string]any{
		"foo":     "",
		"orderby": "",
	})
	require.NoError(t, err)
	assert.Equal(t, "", receivedRawQuery, "empty optional query parameters must not be sent as ?foo=")
}
