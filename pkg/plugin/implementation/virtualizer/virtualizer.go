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

// Invoke posts the payload to Restate at {RestateURL}/{service}.
// It performs a synchronous HTTP POST and returns any transport-level error.
func (v *Virtualizer) Invoke(ctx context.Context, service string, payload []byte) error {
	targetURL := fmt.Sprintf("%s/%s/handle/send", v.cfg.RestateURL, service)
	resp, err := v.client.Post(targetURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("virtualizer: restate invocation failed for %s: %w", targetURL, err)
	}
	defer resp.Body.Close()
	log.Debugf(ctx, "virtualizer: restate invocation %s responded with status %s", targetURL, resp.Status)
	return nil
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

// Run parses context.action from ctx.Body, then fires a fire-and-forget
// goroutine that invokes the Restate service. The step always returns nil so
// the pipeline continues uninterrupted.
func (s *virtualizationStep) Run(ctx *model.StepContext) error {
	action, err := extractAction(ctx.Body)
	if err != nil {
		log.Warnf(ctx, "virtualizer: skipping invocation, failed to extract action: %v", err)
		return nil
	}

	payload := make([]byte, len(ctx.Body))
	copy(payload, ctx.Body)

	go func() {
		if err := s.v.Invoke(ctx, action, payload); err != nil {
			log.Warnf(ctx, "virtualizer: async invocation error: %v", err)
		}
	}()

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
