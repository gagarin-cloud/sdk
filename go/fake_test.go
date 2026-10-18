package gagarin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is a local HTTP server that answers like the gagarin API, from the
// fixtures in ../testdata/api, and records every request it was sent.
type fakeAPI struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	requests []recorded
	// runs, when set, is what successive status reads report as migrate's
	// latest run; the last one repeats. Nil means the fixture's.
	runs []map[string]any
	// statusFailures is how many status reads answer 502 before one succeeds.
	statusFailures int
	healthDown     bool
	nextRevision   int
}

type recorded struct {
	Method, Path, Auth, ContentType string
	// Escaped is the path as sent, before the server decoded it.
	Escaped string
	Query   map[string][]string
	Body    map[string]any
}

func newFake(t *testing.T) (*fakeAPI, *Client) {
	t.Helper()
	f := &fakeAPI{t: t, nextRevision: 8}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	c, err := NewClient(WithToken("tok"), WithBaseURL(f.srv.URL+"/"), WithRegistry("registry.example"))
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "api", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (f *fakeAPI) last() recorded {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		f.t.Fatal("no request was made")
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeAPI) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r.Method == method && r.Path == path {
			n++
		}
	}
	return n
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec := recorded{Method: r.Method, Path: r.URL.Path, Escaped: r.URL.EscapedPath(), Auth: r.Header.Get("Authorization"),
		ContentType: r.Header.Get("Content-Type"), Query: r.URL.Query()}
	if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
		if err := json.Unmarshal(raw, &rec.Body); err != nil {
			f.t.Errorf("%s %s: body is not a JSON object: %s", r.Method, r.URL.Path, raw)
		}
	}
	f.requests = append(f.requests, rec)

	w.Header().Set("Content-Type", "application/json")
	reply := func(status int, body []byte) { w.WriteHeader(status); w.Write(body) }
	fail := func(status int, code, msg string) {
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": msg}})
		reply(status, b)
	}

	// The one public route.
	if r.Method == http.MethodGet && r.URL.Path == "/healthz/platform" {
		if f.healthDown {
			reply(http.StatusServiceUnavailable, fixture(f.t, "health_down.json"))
		} else {
			reply(http.StatusOK, fixture(f.t, "health_ok.json"))
		}
		return
	}
	if rec.Auth != "Bearer tok" {
		reply(http.StatusUnauthorized, fixture(f.t, "error_unauthorized.json"))
		return
	}

	route := r.Method + " " + r.URL.Path
	switch {
	case route == "GET /v1/whoami":
		reply(200, fixture(f.t, "whoami.json"))
	case route == "GET /v1/billing":
		reply(200, fixture(f.t, "billing.json"))
	case route == "GET /v1/projects":
		reply(200, fixture(f.t, "projects.json"))
	case route == "POST /v1/projects":
		if rec.Body["name"] == "shop" {
			reply(409, fixture(f.t, "error_project_exists.json"))
			return
		}
		var p map[string]any
		json.Unmarshal(fixture(f.t, "project_created.json"), &p)
		p["name"] = rec.Body["name"]
		b, _ := json.Marshal(p)
		reply(201, b)
	case route == "GET /v1/projects/shop", route == "GET /v1/projects/p-123":
		reply(200, fixture(f.t, "project.json"))
	case route == "GET /v1/projects/shop/status":
		if f.statusFailures > 0 {
			f.statusFailures--
			fail(502, "cluster_error", "the cluster did not answer")
			return
		}
		var st map[string]any
		json.Unmarshal(fixture(f.t, "status.json"), &st)
		if len(f.runs) > 0 {
			run := f.runs[0]
			if len(f.runs) > 1 {
				f.runs = f.runs[1:]
			}
			for _, s := range st["services"].([]any) {
				if s := s.(map[string]any); s["name"] == "migrate" {
					s["actual"].(map[string]any)["run"] = run
				}
			}
		}
		b, _ := json.Marshal(st)
		reply(200, b)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/projects/shop/services/"):
		if rec.Body["size"] == "xl" {
			reply(400, fixture(f.t, "error_invalid_size.json"))
			return
		}
		out := map[string]any{"project": "p-123", "name": strings.TrimPrefix(r.URL.Path, "/v1/projects/shop/services/"),
			"kind": rec.Body["kind"], "image": rec.Body["image"], "port": 0, "revision": f.nextRevision,
			"created_at": "2026-09-01T12:00:00Z", "updated_at": "2026-10-10T10:00:00Z"}
		for _, k := range []string{"size", "digest", "timeout_seconds", "ttl_seconds", "schedule", "time_zone"} {
			if v, ok := rec.Body[k]; ok {
				out[k] = v
			}
		}
		f.nextRevision++
		b, _ := json.Marshal(out)
		reply(200, b)
	case route == "GET /v1/projects/shop/services/migrate/deployments":
		reply(200, fixture(f.t, "deployments.json"))
	case route == "GET /v1/projects/shop/services/migrate/logs":
		reply(200, fixture(f.t, "logs.json"))
	case strings.HasPrefix(r.URL.Path, "/v1/projects/"):
		fail(404, "not_found", "no project or service by that name")
	default:
		fail(404, "no_such_route", route)
	}
}
