package gagarin

import (
	"context"
	"encoding/json"
	"net/http"
)

// PlatformService says whether gagarin itself is working.
type PlatformService struct{ c *Client }

// Check is one part of the platform's health.
type Check struct {
	OK bool `json:"ok"`
	// Detail says what is wrong, when something is.
	Detail string `json:"detail,omitempty"`
	// Millis is how long the check took.
	Millis int64 `json:"ms"`
}

// Health is the platform's own answer to "is gagarin working".
type Health struct {
	OK         bool   `json:"ok"`
	Serving    Check  `json:"serving"`
	Cluster    *Check `json:"cluster,omitempty"`
	Reconciler *Check `json:"reconciler,omitempty"`
}

// Health reads the platform's health. An unhealthy platform is an answer,
// not an error: read OK and each check's Detail. An error means the API
// could not be asked at all.
func (p *PlatformService) Health(ctx context.Context) (*Health, error) {
	status, raw, err := p.c.send(ctx, http.MethodGet, "/healthz/platform", nil, nil)
	if err != nil {
		return nil, err
	}
	var out Health
	if (status == http.StatusOK || status == http.StatusServiceUnavailable) && json.Unmarshal(raw, &out) == nil {
		return &out, nil
	}
	return nil, apiError(status, raw)
}
