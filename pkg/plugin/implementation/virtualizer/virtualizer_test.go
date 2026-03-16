package virtualizer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/beckn-one/beckn-onix/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew_MissingRestateURL verifies New returns an error when restate_url is empty.
func TestNew_MissingRestateURL(t *testing.T) {
	_, _, err := New(context.Background(), &Config{RestateURL: ""})
	assert.Error(t, err)
}

// TestNew_ValidConfig verifies New succeeds and the closer is callable.
func TestNew_ValidConfig(t *testing.T) {
	v, closer, err := New(context.Background(), &Config{RestateURL: "http://localhost:8080"})
	require.NoError(t, err)
	assert.NotNil(t, v)
	assert.NotNil(t, closer)
	closer()
}

// TestInvoke_PostsToCorrectURL verifies Invoke sends a synchronous POST to {RestateURL}/{service}.
func TestInvoke_PostsToCorrectURL(t *testing.T) {
	responseBody := []byte(`{"context":{"action":"search"},"message":{}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody) //nolint:errcheck
	}))
	defer srv.Close()

	v, closer, err := New(context.Background(), &Config{RestateURL: srv.URL})
	require.NoError(t, err)
	defer closer()

	payload := []byte(`{"context":{"action":"search"}}`)
	result, err := v.Invoke(context.Background(), "search", payload)
	assert.NoError(t, err)
	assert.Equal(t, responseBody, result)
}

// TestInvoke_ServerError verifies Invoke returns an error on transport failure.
func TestInvoke_ServerError(t *testing.T) {
	v, closer, err := New(context.Background(), &Config{RestateURL: "http://127.0.0.1:1"})
	require.NoError(t, err)
	defer closer()

	_, err = v.Invoke(context.Background(), "search", []byte(`{}`))
	assert.Error(t, err)
}

// TestInvoke_NonSuccessStatus verifies Invoke returns an error on non-2xx HTTP status.
func TestInvoke_NonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	v, closer, err := New(context.Background(), &Config{RestateURL: srv.URL})
	require.NoError(t, err)
	defer closer()

	_, err = v.Invoke(context.Background(), "search", []byte(`{}`))
	assert.Error(t, err)
}

// TestExtractAction_Valid verifies extractAction returns the correct action string.
func TestExtractAction_Valid(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"action": "select"},
	})
	action, err := extractAction(body)
	require.NoError(t, err)
	assert.Equal(t, "select", action)
}

// TestExtractAction_MissingContext verifies extractAction errors when context is absent.
func TestExtractAction_MissingContext(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{"message": "hello"})
	_, err := extractAction(body)
	assert.Error(t, err)
}

// TestExtractAction_MissingAction verifies extractAction errors when context.action is absent.
func TestExtractAction_MissingAction(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"domain": "ONDC:TRV10"},
	})
	_, err := extractAction(body)
	assert.Error(t, err)
}

// TestExtractAction_InvalidJSON verifies extractAction errors on malformed JSON.
func TestExtractAction_InvalidJSON(t *testing.T) {
	_, err := extractAction([]byte(`not-json`))
	assert.Error(t, err)
}

// TestNewStep_Run_SyncTransform verifies the step synchronously invokes Restate
// and replaces ctx.Body with the transformed payload.
func TestNewStep_Run_SyncTransform(t *testing.T) {
	transformedBody := []byte(`{"context":{"action":"confirm","version":"2.0.0"},"message":{}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/confirm", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.NotEmpty(t, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(transformedBody) //nolint:errcheck
	}))
	defer srv.Close()

	v, closer, err := New(context.Background(), &Config{RestateURL: srv.URL})
	require.NoError(t, err)
	defer closer()

	step := NewStep(v)

	original, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"action": "confirm"},
	})
	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    original,
	}

	err = step.Run(ctx)
	assert.NoError(t, err)
	assert.Equal(t, transformedBody, ctx.Body, "ctx.Body must be replaced with transformed payload")
}

// TestNewStep_Run_MissingAction verifies the step skips invocation and returns nil
// when context.action is absent from the body.
func TestNewStep_Run_MissingAction(t *testing.T) {
	v, closer, err := New(context.Background(), &Config{RestateURL: "http://localhost:8080"})
	require.NoError(t, err)
	defer closer()

	step := NewStep(v)

	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{},
	})
	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    body,
	}

	err = step.Run(ctx)
	assert.NoError(t, err, "Run must return nil when action is missing")
	assert.Equal(t, body, ctx.Body, "ctx.Body must be unchanged when action is missing")
}

// TestNewStep_Run_RestateError verifies the step propagates Restate errors.
func TestNewStep_Run_RestateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	v, closer, err := New(context.Background(), &Config{RestateURL: srv.URL})
	require.NoError(t, err)
	defer closer()

	step := NewStep(v)
	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"action": "search"},
	})
	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    body,
	}

	err = step.Run(ctx)
	assert.Error(t, err, "Run must return error when Restate fails")
}
