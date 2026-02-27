package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestStep_Run_FireAndForget verifies the step fires a POST to Restate and
// returns nil immediately (fire-and-forget), without blocking the pipeline.
func TestStep_Run_FireAndForget(t *testing.T) {
	received := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	step, closer, err := Provider.New(context.Background(), map[string]string{
		"restate_url": srv.URL,
	})
	require.NoError(t, err)
	defer closer()

	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{
			"action": "search",
		},
	})

	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    body,
		SubID:   "bpp1",
		Role:    model.RoleBPP,
	}

	err = step.Run(ctx)
	assert.NoError(t, err, "Run should return nil immediately")

	// Wait for the async invocation with a timeout.
	select {
	case req := <-received:
		assert.Equal(t, "/bpp1/search", req.URL.Path)
		assert.Equal(t, http.MethodPost, req.Method)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Restate invocation")
	}
}

// TestStep_Run_MissingAction verifies the step returns nil (skips invocation)
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
	assert.NoError(t, err, "Run should return nil even when action is missing")
}

// TestStep_Run_BAPDeployment verifies the deployment is set to ctx.SubID for BAP role.
func TestStep_Run_BAPDeployment(t *testing.T) {
	received := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	step, closer, err := Provider.New(context.Background(), map[string]string{
		"restate_url": srv.URL,
	})
	require.NoError(t, err)
	defer closer()

	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{
			"action": "on_search",
		},
	})

	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    body,
		SubID:   "bap1",
		Role:    model.RoleBAP,
	}

	err = step.Run(ctx)
	assert.NoError(t, err)

	select {
	case req := <-received:
		assert.Equal(t, "/bap1/on_search", req.URL.Path)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Restate invocation")
	}
}
