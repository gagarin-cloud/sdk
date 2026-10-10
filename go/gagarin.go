// Package gagarin is a client for the gagarin API.
//
// Its first job is the one `gg run` does: start a job from an image that is
// already in gagarin's registry, and optionally wait for it to finish.
//
//	c, err := gagarin.NewClient() // GAGARIN_TOKEN, GAGARIN_API
//	res, err := c.Jobs.RunAndWait(ctx, "shop", "migrate", gagarin.RunOptions{
//		Image: "migrate:v3",
//		Deps:  []string{"db"},
//	})
//	os.Exit(res.ExitCode())
package gagarin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the gagarin API.
	DefaultBaseURL = "https://api.gagarin.cloud"
	// DefaultRegistry is where gagarin runs images from.
	DefaultRegistry = "registry.gagarin.cloud"
)

// Client talks to the gagarin API. Build one with NewClient.
type Client struct {
	baseURL  string
	token    string
	registry string
	http     *http.Client

	// Jobs submits, waits on and reads jobs.
	Jobs *JobsService
}

// Option configures a Client.
type Option func(*Client)

// WithToken sets the credential. Defaults to $GAGARIN_TOKEN.
func WithToken(token string) Option { return func(c *Client) { c.token = token } }

// WithBaseURL sets the API address. Defaults to $GAGARIN_API, then DefaultBaseURL.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") } }

// WithRegistry sets the registry host a bare image name is expanded against.
// Defaults to $GAGARIN_REGISTRY, then DefaultRegistry.
func WithRegistry(host string) Option {
	return func(c *Client) { c.registry = strings.TrimRight(host, "/") }
}

// WithHTTPClient replaces the http.Client used for every request.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// NewClient builds a Client. It fails only when no token was given and
// $GAGARIN_TOKEN is empty.
func NewClient(opts ...Option) (*Client, error) {
	c := &Client{
		baseURL:  strings.TrimRight(envOr("GAGARIN_API", DefaultBaseURL), "/"),
		token:    os.Getenv("GAGARIN_TOKEN"),
		registry: strings.TrimRight(envOr("GAGARIN_REGISTRY", DefaultRegistry), "/"),
		http:     &http.Client{Timeout: 2 * time.Minute},
	}
	for _, o := range opts {
		o(c)
	}
	if c.token == "" {
		return nil, errors.New("gagarin: no token: pass WithToken or set GAGARIN_TOKEN")
	}
	c.Jobs = &JobsService{c: c}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Error is what the API answers when it refuses a request. Branch on Code,
// which is stable; Message and FixHint are prose for a human.
type Error struct {
	StatusCode int    `json:"-"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	FixHint    string `json:"fix_hint,omitempty"`
}

func (e *Error) Error() string {
	s := fmt.Sprintf("gagarin: [%s] %s", e.Code, e.Message)
	if e.FixHint != "" {
		s += "\nhint: " + e.FixHint
	}
	return s
}

// IsCode reports whether err is an *Error with the given code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "gagarin-sdk-go")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var env struct {
			Error *Error `json:"error"`
		}
		if json.Unmarshal(raw, &env) == nil && env.Error != nil && env.Error.Code != "" {
			env.Error.StatusCode = resp.StatusCode
			return env.Error
		}
		return &Error{StatusCode: resp.StatusCode, Code: "http_" + fmt.Sprint(resp.StatusCode),
			Message: strings.TrimSpace(string(raw))}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Project is a project as GET /v1/projects/{project} answers it.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Project resolves a project name or id.
func (c *Client) Project(ctx context.Context, project string) (*Project, error) {
	var p Project
	if err := c.do(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(project), nil, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ImageRef expands a bare repository reference ("migrate:v3") into the full
// one gagarin runs from ("registry.gagarin.cloud/<project-id>/migrate:v3"). A
// reference that already contains a "/" is returned unchanged.
func (c *Client) ImageRef(ctx context.Context, project, image string) (string, error) {
	if strings.Contains(image, "/") {
		return image, nil
	}
	p, err := c.Project(ctx, project)
	if err != nil {
		return "", err
	}
	return c.registry + "/" + p.ID + "/" + image, nil
}
