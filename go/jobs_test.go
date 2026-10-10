package gagarin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeAPI is just enough of the gagarin API to submit a job and watch it run.
type fakeAPI struct {
	mu       sync.Mutex
	put      map[string]any
	auth     string
	phases   []string // what successive status reads report for revision 7
	statuses int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = r.Header.Get("Authorization")
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/shop":
		json.NewEncoder(w).Encode(map[string]any{"id": "p-123", "name": "shop"})
	case r.Method == http.MethodPut && r.URL.Path == "/v1/projects/shop/services/migrate":
		json.NewDecoder(r.Body).Decode(&f.put)
		if f.put["size"] == "xl" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"code":"invalid_size","message":"size \"xl\" is not one of s, m, l","fix_hint":"size is one of s, m, l"}}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"project": "p-123", "name": "migrate", "kind": "job",
			"image": f.put["image"], "revision": 7})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/shop/status":
		i := f.statuses
		if i >= len(f.phases) {
			i = len(f.phases) - 1
		}
		f.statuses++
		run := map[string]any{"revision": 7, "phase": f.phases[i]}
		if f.phases[i] == PhaseDone || f.phases[i] == PhaseFailed {
			code := 0
			if f.phases[i] == PhaseFailed {
				code = 3
			}
			run["exit_code"] = code
		}
		if i == 0 {
			run["revision"] = 6 // the previous run, still the latest the cluster reports
			run["phase"] = PhaseDone
		}
		json.NewEncoder(w).Encode(map[string]any{"services": []any{
			map[string]any{"name": "web", "kind": "container"},
			map[string]any{"name": "migrate", "kind": "job", "actual": map[string]any{"exists": true, "run": run}},
		}})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/shop/services/migrate/logs":
		json.NewEncoder(w).Encode(map[string]any{"logs": "q=" + r.URL.Query().Get("q") + "\n", "source": "store"})
	default:
		http.NotFound(w, r)
	}
}

func newTest(t *testing.T, phases ...string) (*Client, *fakeAPI) {
	t.Helper()
	f := &fakeAPI{phases: phases}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := NewClient(WithToken("tok"), WithBaseURL(srv.URL), WithRegistry("registry.example"))
	if err != nil {
		t.Fatal(err)
	}
	return c, f
}

func TestRunSendsEverySetting(t *testing.T) {
	c, f := newTest(t, PhasePending)
	sub, err := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{
		Image: "migrate:v3", Digest: "sha256:abc", Env: map[string]string{"A": "1"},
		Deps: []string{"db"}, Size: SizeM, TimeoutSeconds: 600, Schedule: "@daily", TimeZone: "Europe/Berlin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Revision != 7 || sub.Kind != "job" {
		t.Fatalf("submission = %+v", sub)
	}
	want := map[string]any{
		"kind": "job", "image": "registry.example/p-123/migrate:v3", "digest": "sha256:abc",
		"env": map[string]any{"A": "1"}, "deps": []any{"db"}, "size": "m",
		"timeout_seconds": float64(600), "schedule": "@daily", "time_zone": "Europe/Berlin",
	}
	got, _ := json.Marshal(f.put)
	exp, _ := json.Marshal(want)
	if string(got) != string(exp) {
		t.Fatalf("body\n got %s\nwant %s", got, exp)
	}
	if f.auth != "Bearer tok" {
		t.Fatalf("auth = %q", f.auth)
	}
}

func TestRunOmitsUnsetSettings(t *testing.T) {
	c, f := newTest(t, PhasePending)
	if _, err := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{Image: "registry.gagarin.cloud/p-123/m:1"}); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(f.put)
	if string(got) != `{"env":{},"image":"registry.gagarin.cloud/p-123/m:1","kind":"job"}` {
		t.Fatalf("body = %s", got)
	}
}

func TestRunAndWaitFollowsTheRevision(t *testing.T) {
	c, _ := newTest(t, "", PhasePending, PhaseRunning, PhaseFailed)
	var seen []string
	res, err := c.Jobs.RunAndWait(context.Background(), "shop", "migrate", RunOptions{Image: "m:1"},
		WaitOptions{PollInterval: time.Millisecond, OnUpdate: func(r *RunState) { seen = append(seen, r.Phase) }})
	if err != nil {
		t.Fatal(err)
	}
	if res.Run.Revision != 7 || res.Run.Phase != PhaseFailed || res.ExitCode() != 3 {
		t.Fatalf("run = %+v", res.Run)
	}
	if len(seen) != 3 || seen[2] != PhaseFailed {
		t.Fatalf("updates = %v", seen)
	}
}

func TestRunAndWaitSucceeds(t *testing.T) {
	c, _ := newTest(t, "", PhaseDone)
	res, err := c.Jobs.RunAndWait(context.Background(), "shop", "migrate", RunOptions{Image: "m:1"},
		WaitOptions{PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Run.Succeeded() || res.ExitCode() != 0 {
		t.Fatalf("run = %+v", res.Run)
	}
}

func TestRunAndWaitRefusesSchedule(t *testing.T) {
	c, _ := newTest(t, PhasePending)
	if _, err := c.Jobs.RunAndWait(context.Background(), "shop", "migrate", RunOptions{Image: "m:1", Schedule: "@daily"}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAPIErrorIsTyped(t *testing.T) {
	c, _ := newTest(t, PhasePending)
	_, err := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{Image: "m:1", Size: "xl"})
	var e *Error
	if !errors.As(err, &e) || e.Code != "invalid_size" || e.StatusCode != 400 || e.FixHint == "" {
		t.Fatalf("err = %#v", err)
	}
	if !IsCode(err, "invalid_size") {
		t.Fatal("IsCode")
	}
}

func TestGetRefusesAService(t *testing.T) {
	c, _ := newTest(t, PhasePending)
	if _, err := c.Jobs.Get(context.Background(), "shop", "web"); !IsCode(err, "not_a_job") {
		t.Fatalf("err = %v", err)
	}
}

func TestLogs(t *testing.T) {
	c, _ := newTest(t, PhasePending)
	l, err := c.Jobs.Logs(context.Background(), "shop", "migrate", LogsOptions{Contains: "boom"})
	if err != nil || l.Logs != "q=boom\n" {
		t.Fatalf("logs = %+v, %v", l, err)
	}
}

func TestNewClientNeedsAToken(t *testing.T) {
	t.Setenv("GAGARIN_TOKEN", "")
	if _, err := NewClient(); err == nil {
		t.Fatal("expected an error")
	}
}
