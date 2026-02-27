package main

import (
	"context"
	"errors"

	"github.com/beckn-one/beckn-onix/pkg/log"
	"github.com/beckn-one/beckn-onix/pkg/plugin/definition"
	"github.com/beckn-one/beckn-onix/pkg/plugin/implementation/virtualizer"
)

// virtualizerProvider implements definition.StepProvider.
type virtualizerProvider struct{}

// New creates a new virtualization step from the given config map.
// Expected config keys:
//   - restate_url: base URL of the Restate server (e.g. "http://localhost:8080")
func (p virtualizerProvider) New(ctx context.Context, config map[string]string) (definition.Step, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("context cannot be nil")
	}
	cfg := &virtualizer.Config{
		RestateURL: config["restate_url"],
	}
	log.Debugf(ctx, "Virtualizer config: restate_url=%s", cfg.RestateURL)
	v, closer, err := virtualizer.New(ctx, cfg)
	if err != nil {
		log.Errorf(ctx, err, "Failed to create virtualizer instance")
		return nil, nil, err
	}
	log.Infof(ctx, "Virtualizer step created successfully")
	return virtualizer.NewStep(v), closer, nil
}

// Provider is the exported plugin symbol loaded by the plugin manager.
var Provider = virtualizerProvider{}
