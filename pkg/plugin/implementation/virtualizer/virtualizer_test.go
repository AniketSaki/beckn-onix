package virtualizer

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

// TestInvoke_PostsToCorrectURL verifies Invoke sends a POST to {RestateURL}/{deployment}/{service}.
func TestInvoke_PostsToCorrectURL(t *testing.T) {
	received := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	v, closer, err := New(context.Background(), &Config{RestateURL: srv.URL})
	require.NoError(t, err)
	defer closer()

	payload := []byte(`{"context":{"action":"search"}}`)
	err = v.Invoke(context.Background(), "bpp1", "search", payload)
	assert.NoError(t, err)

	select {
	case req := <-received:
		assert.Equal(t, "/bpp1/search", req.URL.Path)
		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Restate invocation")
	}
}

// TestInvoke_ServerError verifies Invoke returns an error on transport failure.
func TestInvoke_ServerError(t *testing.T) {
	v, closer, err := New(context.Background(), &Config{RestateURL: "http://127.0.0.1:1"})
	require.NoError(t, err)
	defer closer()

	err = v.Invoke(context.Background(), "bpp1", "search", []byte(`{}`))
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

// TestNewStep_Run_FireAndForget verifies the step fires a goroutine and returns nil.
func TestNewStep_Run_FireAndForget(t *testing.T) {
	received := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	v, closer, err := New(context.Background(), &Config{RestateURL: srv.URL})
	require.NoError(t, err)
	defer closer()

	step := NewStep(v)

	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"action": "confirm"},
	})
	ctx := &model.StepContext{
		Context: context.Background(),
		Body:    body,
		SubID:   "bpp1",
		Role:    model.RoleBPP,
	}

	err = step.Run(ctx)
	assert.NoError(t, err, "Run must return nil immediately")

	select {
	case req := <-received:
		assert.Equal(t, "/bpp1/confirm", req.URL.Path)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Restate invocation")
	}
}

// TestNewStep_Run_BAPRole verifies deployment uses ctx.SubID for BAP role.
func TestNewStep_Run_BAPRole(t *testing.T) {
	received := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	v, closer, err := New(context.Background(), &Config{RestateURL: srv.URL})
	require.NoError(t, err)
	defer closer()

	step := NewStep(v)

	body, _ := json.Marshal(map[string]interface{}{
		"context": map[string]interface{}{"action": "on_search"},
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
		SubID:   "bpp1",
		Role:    model.RoleBPP,
	}

	err = step.Run(ctx)
	assert.NoError(t, err, "Run must return nil even when action is missing")
}
