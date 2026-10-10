package gagarin

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ServicesService reads what any service or job has printed and been.
type ServicesService struct{ c *Client }

// LogsOptions narrows a read of logs. All optional.
type LogsOptions struct {
	// Limit is how many lines, newest last.
	Limit int
	// Since and Until are a duration back from now ("30m", "6h", "2d") or
	// an RFC 3339 time. Until is usually the Next of the previous page.
	Since, Until string
	// Contains keeps only lines containing this text.
	Contains string
	// Previous reads the last crashed container's output instead.
	Previous bool
}

// Logs is a page of log lines.
type Logs struct {
	// Logs is the lines, oldest first, newline-terminated.
	Logs string `json:"logs"`
	// Source is "store" or "cluster".
	Source string `json:"source"`
	// Next, when set, is the Until that reads the page before this one.
	Next *string `json:"next"`
	// Notice says why the answer is less than what was asked for, if it is.
	Notice string `json:"notice,omitempty"`
}

// Logs reads a service's or job's logs.
func (s *ServicesService) Logs(ctx context.Context, project, name string, opts LogsOptions) (*Logs, error) {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.Until != "" {
		q.Set("until", opts.Until)
	}
	if opts.Contains != "" {
		q.Set("q", opts.Contains)
	}
	if opts.Previous {
		q.Set("previous", "true")
	}
	var out Logs
	if err := s.c.do(ctx, http.MethodGet, servicePath(project, name)+"/logs", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Deployment is one recorded deploy or run: what the service was asked to
// be at that moment. Environment values are never returned, only names.
type Deployment struct {
	Revision       int       `json:"revision"`
	Project        string    `json:"project"`
	Service        string    `json:"service"`
	Image          string    `json:"image"`
	Digest         string    `json:"digest,omitempty"`
	Port           int       `json:"port"`
	Public         bool      `json:"public"`
	URL            string    `json:"url,omitempty"`
	Needs          []string  `json:"needs"`
	EnvKeys        []string  `json:"env_keys"`
	VolumePath     string    `json:"volume_path,omitempty"`
	VolumeSizeGB   int       `json:"volume_size_gb,omitempty"`
	TimeoutSeconds int       `json:"timeout_seconds,omitempty"`
	Schedule       string    `json:"schedule,omitempty"`
	TimeZone       string    `json:"time_zone,omitempty"`
	DeployedBy     string    `json:"deployed_by,omitempty"`
	RestoredFrom   *int      `json:"restored_from,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// Deployments lists a service's or job's revisions, newest first.
func (s *ServicesService) Deployments(ctx context.Context, project, name string) ([]Deployment, error) {
	var out struct {
		Deployments []Deployment `json:"deployments"`
	}
	if err := s.c.do(ctx, http.MethodGet, servicePath(project, name)+"/deployments", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Deployments, nil
}
