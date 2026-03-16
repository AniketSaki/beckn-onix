package main

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

// TestProviderVariable verifies the exported Provider symbol is non-nil.
func TestProviderVariable(t *testing.T) {
	assert.NotNil(t, Provider, "Provider should not be nil")
}

// TestProviderNew_NilContext ensures New returns an error when context is nil.
func TestProviderNew_NilContext(t *testing.T) {
	step, closer, err := Provider.New(nil, map[string]string{"restate_url": "http://localhost:8080"})
	assert.Error(t, err)
	assert.Nil(t, step)
	assert.Nil(t, closer)
}

// TestProviderNew_MissingRestateURL ensures New returns an error when restate_url is absent.
func TestProviderNew_MissingRestateURL(t *testing.T) {
	step, closer, err := Provider.New(context.Background(), map[string]string{})
	assert.Error(t, err)
	assert.Nil(t, step)
	assert.Nil(t, closer)
}

// TestProviderNew_ValidConfig ensures New succeeds and returns a usable Step.
func TestProviderNew_ValidConfig(t *testing.T) {
	step, closer, err := Provider.New(context.Background(), map[string]string{
		"restate_url": "http://localhost:8080",
	})
	require.NoError(t, err)
	assert.NotNil(t, step)
	assert.NotNil(t, closer)
	closer()
}

// TestStep_Run_SyncTransform verifies the step synchronously invokes Restate and
// replaces ctx.Body with the transformed payload returned by the service.
func TestStep_Run_SyncTransform(t *testing.T) {
	transformedBody := []byte(`{"context":{"action":"search","version":"2.0.0"},"message":{}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/search", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		assert.NotEmpty(t, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(transformedBody) //nolint:errcheck
	}))
	defer srv.Close()

	step, closer, err := Provider.New(context.Background(), map[string]string{
		"restate_url": srv.URL,
	})
	require.NoError(t, err)
	defer closer()

	original, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"action": "search"},
	})
	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    original,
		SubID:   "bpp1",
		Role:    model.RoleBPP,
	}

	err = step.Run(ctx)
	assert.NoError(t, err)
	assert.Equal(t, transformedBody, ctx.Body, "ctx.Body must be replaced with transformed payload")
}

// TestStep_Run_MissingAction verifies the step returns nil and leaves ctx.Body unchanged
// when the request body does not contain context.action.
func TestStep_Run_MissingAction(t *testing.T) {
	step, closer, err := Provider.New(context.Background(), map[string]string{
		"restate_url": "http://localhost:8080",
	})
	require.NoError(t, err)
	defer closer()

	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{
			// action intentionally omitted
		},
	})

	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    body,
		SubID:   "bpp1",
		Role:    model.RoleBPP,
	}

	err = step.Run(ctx)
	assert.NoError(t, err, "Run should return nil when action is missing")
	assert.Equal(t, body, ctx.Body, "ctx.Body must be unchanged when action is missing")
}

// TestStep_Run_RestateError verifies the step returns an error when Restate responds with non-2xx.
func TestStep_Run_RestateError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	step, closer, err := Provider.New(context.Background(), map[string]string{
		"restate_url": srv.URL,
	})
	require.NoError(t, err)
	defer closer()

	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"action": "search"},
	})
	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    body,
	}

	err = step.Run(ctx)
	assert.Error(t, err, "Run must propagate Restate errors")
}
