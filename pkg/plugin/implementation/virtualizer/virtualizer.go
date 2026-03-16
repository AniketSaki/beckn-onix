package virtualizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/beckn-one/beckn-onix/pkg/log"
	"github.com/beckn-one/beckn-onix/pkg/model"
	"github.com/beckn-one/beckn-onix/pkg/plugin/definition"
)

// Config holds the virtualizer plugin configuration.
type Config struct {
	RestateURL string // e.g. "http://localhost:8080"
}

// Virtualizer implements definition.Virtualizer by posting payloads to a
// Restate server endpoint: POST {RestateURL}/{deployment}/{service}
type Virtualizer struct {
	cfg    *Config
	client *http.Client
}

// New initializes a Virtualizer instance. Returns the instance, a no-op closer,
// and any configuration error.
func New(ctx context.Context, cfg *Config) (*Virtualizer, func(), error) {
	if cfg.RestateURL == "" {
		return nil, nil, fmt.Errorf("virtualizer: restate_url is required")
	}
	log.Infof(ctx, "Virtualizer initialized with restate_url: %s", cfg.RestateURL)
	return &Virtualizer{
		cfg:    cfg,
		client: &http.Client{},
	}, func() {}, nil
}

// Invoke posts the payload to Restate at {RestateURL}/{service} (synchronous /invoke).
// It waits for the response and returns the transformed payload body.
func (v *Virtualizer) Invoke(ctx context.Context, service string, payload []byte) ([]byte, error) {
	targetURL := fmt.Sprintf("%s/%s", v.cfg.RestateURL, service)
	resp, err := v.client.Post(targetURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("virtualizer: restate invocation failed for %s: %w", targetURL, err)
	}
	defer resp.Body.Close()
	log.Debugf(ctx, "virtualizer: restate invocation %s responded with status %s", targetURL, resp.Status)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("virtualizer: restate returned non-2xx status %s for %s", resp.Status, targetURL)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("virtualizer: failed to read response body from %s: %w", targetURL, err)
	}
	return buf.Bytes(), nil
}

// Ensure Virtualizer satisfies the definition.Virtualizer interface at compile time.
var _ definition.Virtualizer = (*Virtualizer)(nil)

// virtualizationStep implements definition.Step by wrapping a Virtualizer.
type virtualizationStep struct {
	v definition.Virtualizer
}

// NewStep creates a Step that invokes the given Virtualizer on each request.
func NewStep(v definition.Virtualizer) definition.Step {
	return &virtualizationStep{v: v}
}

// Run parses context.action from ctx.Body, synchronously invokes the Restate
// service, and replaces ctx.Body with the transformed payload so subsequent
// pipeline steps (schema validation, routing) operate on the transformed request.
func (s *virtualizationStep) Run(ctx *model.StepContext) error {
	action, err := extractAction(ctx.Body)
	if err != nil {
		log.Warnf(ctx, "virtualizer: skipping invocation, failed to extract action: %v", err)
		return nil
	}

	transformed, err := s.v.Invoke(ctx, action, ctx.Body)
	if err != nil {
		return fmt.Errorf("virtualizer: invocation failed: %w", err)
	}

	ctx.Body = transformed
	return nil
}

// extractAction parses the beckn JSON body and returns context.action.
func extractAction(body []byte) (string, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", fmt.Errorf("failed to parse request body: %w", err)
	}
	ctxObj, ok := req["context"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("context field missing or invalid")
	}
	action, ok := ctxObj["action"].(string)
	if !ok || action == "" {
		return "", fmt.Errorf("context.action field missing or invalid")
	}
	return action, nil
}
