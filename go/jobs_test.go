package gagarin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func run(revision int, phase string, exit ...int) map[string]any {
	r := map[string]any{"revision": revision, "phase": phase}
	if len(exit) > 0 {
		r["exit_code"] = exit[0]
	}
	return r
}

var fast = WaitOptions{PollInterval: time.Millisecond}

func TestRunSendsEverySetting(t *testing.T) {
	f, c := newFake(t)
	sub, err := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{
		Image: "migrate:v3", Digest: "sha256:abc", Env: map[string]string{"A": "1"}, Deps: []string{"db"},
		Size: SizeM, TimeoutSeconds: 600, TTLSeconds: 3600, Schedule: "@daily", TimeZone: "Europe/Berlin",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := f.last()
	if r.Method != "PUT" || r.Path != "/v1/projects/shop/services/migrate" || r.ContentType != "application/json" {
		t.Fatalf("request = %s %s (%s)", r.Method, r.Path, r.ContentType)
	}
	want := map[string]any{
		"kind": "job", "image": "registry.example/p-123/migrate:v3", "digest": "sha256:abc",
		"env": map[string]any{"A": "1"}, "deps": []any{"db"}, "size": "m",
		"timeout_seconds": float64(600), "ttl_seconds": float64(3600), "schedule": "@daily", "time_zone": "Europe/Berlin",
	}
	got, _ := json.Marshal(r.Body)
	exp, _ := json.Marshal(want)
	if string(got) != string(exp) {
		t.Fatalf("body\n got %s\nwant %s", got, exp)
	}
	if sub.Revision != 8 || sub.Kind != "job" || sub.Image != "registry.example/p-123/migrate:v3" || sub.Schedule != "@daily" || sub.TTLSeconds != 3600 {
		t.Fatalf("submission = %+v", sub)
	}
}

func TestRunOmitsUnsetSettingsButAlwaysSendsEnv(t *testing.T) {
	f, c := newFake(t)
	if _, err := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{Image: "registry.gagarin.cloud/p-123/m:1"}); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(f.last().Body)
	if string(got) != `{"env":{},"image":"registry.gagarin.cloud/p-123/m:1","kind":"job"}` {
		t.Fatalf("body = %s", got)
	}
	if f.count("GET", "/v1/projects/shop") != 0 {
		t.Fatal("a full image reference should not look the project up")
	}
}

func TestRunNeedsAnImage(t *testing.T) {
	f, c := newFake(t)
	if _, err := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{}); err == nil {
		t.Fatal("expected an error")
	}
	if len(f.requests) != 0 {
		t.Fatal("nothing should be sent without an image")
	}
}

func TestRunRefusalIsTyped(t *testing.T) {
	_, c := newFake(t)
	_, err := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{Image: "m:1", Size: "xl"})
	var e *Error
	if !errors.As(err, &e) || e.Code != "invalid_size" || e.StatusCode != 400 || e.FixHint == "" {
		t.Fatalf("err = %#v", err)
	}
}

func TestEachRunIsANewRevision(t *testing.T) {
	_, c := newFake(t)
	a, _ := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{Image: "m:1"})
	b, _ := c.Jobs.Run(context.Background(), "shop", "migrate", RunOptions{Image: "m:1"})
	if b.Revision != a.Revision+1 {
		t.Fatalf("revisions %d then %d", a.Revision, b.Revision)
	}
}

func TestGetReadsTheJobsRow(t *testing.T) {
	_, c := newFake(t)
	j, err := c.Jobs.Get(context.Background(), "shop", "migrate")
	if err != nil || j.Actual.Run.Revision != 7 || j.Actual.Run.Phase != PhaseFailed {
		t.Fatalf("job = %+v, %v", j, err)
	}
	if _, err := c.Jobs.Get(context.Background(), "shop", "web"); !IsCode(err, "not_a_job") {
		t.Fatalf("a service: err = %v", err)
	}
	if _, err := c.Jobs.Get(context.Background(), "shop", "nope"); !IsCode(err, "not_found") {
		t.Fatalf("a missing name: err = %v", err)
	}
}

func TestWaitIgnoresThePreviousRunAndReportsEachPhase(t *testing.T) {
	f, c := newFake(t)
	// The status still shows run 7 (finished) right after run 8 was submitted.
	f.runs = []map[string]any{run(7, PhaseDone, 0), run(8, PhasePending), run(8, PhaseRunning), run(8, PhaseRunning), run(8, PhaseFailed, 3)}
	var seen []string
	opts := fast
	opts.OnUpdate = func(r *RunState) { seen = append(seen, r.Phase) }
	r, err := c.Jobs.Wait(context.Background(), "shop", "migrate", 8, opts)
	if err != nil {
		t.Fatal(err)
	}
	if r.Revision != 8 || r.Phase != PhaseFailed || *r.ExitCode != 3 || r.Succeeded() {
		t.Fatalf("run = %+v", r)
	}
	if len(seen) != 3 || seen[0] != PhasePending || seen[1] != PhaseRunning || seen[2] != PhaseFailed {
		t.Fatalf("updates = %v (each phase once)", seen)
	}
}

func TestWaitWithNoRunYet(t *testing.T) {
	f, c := newFake(t)
	f.runs = []map[string]any{nil, nil, run(8, PhaseDone, 0)}
	r, err := c.Jobs.Wait(context.Background(), "shop", "migrate", 8, fast)
	if err != nil || !r.Succeeded() {
		t.Fatalf("run = %+v, %v", r, err)
	}
}

func TestWaitRidesOutTransientFailures(t *testing.T) {
	f, c := newFake(t)
	f.statusFailures = 3
	f.runs = []map[string]any{run(8, PhaseDone, 0)}
	if _, err := c.Jobs.Wait(context.Background(), "shop", "migrate", 8, fast); err != nil {
		t.Fatalf("three 502s should be retried: %v", err)
	}
}

func TestWaitGivesUpOnPersistentFailures(t *testing.T) {
	f, c := newFake(t)
	f.statusFailures = 100
	if _, err := c.Jobs.Wait(context.Background(), "shop", "migrate", 8, fast); !IsCode(err, "cluster_error") {
		t.Fatalf("err = %v", err)
	}
	if n := f.count("GET", "/v1/projects/shop/status"); n != 6 {
		t.Fatalf("status was read %d times, want 6 (one try and five retries)", n)
	}
}

func TestWaitDoesNotRetryARefusal(t *testing.T) {
	f, c := newFake(t)
	if _, err := c.Jobs.Wait(context.Background(), "shop", "web", 8, fast); !IsCode(err, "not_a_job") {
		t.Fatalf("err = %v", err)
	}
	if n := f.count("GET", "/v1/projects/shop/status"); n != 1 {
		t.Fatalf("status was read %d times", n)
	}
}

func TestWaitSuperseded(t *testing.T) {
	f, c := newFake(t)
	f.runs = []map[string]any{run(9, PhaseRunning)}
	r, err := c.Jobs.Wait(context.Background(), "shop", "migrate", 8, fast)
	if !errors.Is(err, ErrSuperseded) || r.Revision != 9 {
		t.Fatalf("run = %+v, err = %v", r, err)
	}
}

func TestWaitSuspended(t *testing.T) {
	f, c := newFake(t)
	f.runs = []map[string]any{run(8, PhaseSuspended)}
	if _, err := c.Jobs.Wait(context.Background(), "shop", "migrate", 8, fast); !errors.Is(err, ErrSuspended) {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitTimesOut(t *testing.T) {
	f, c := newFake(t)
	f.runs = []map[string]any{run(8, PhaseRunning)}
	opts := fast
	opts.Timeout = 20 * time.Millisecond
	if _, err := c.Jobs.Wait(context.Background(), "shop", "migrate", 8, opts); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitHonoursCancellation(t *testing.T) {
	f, c := newFake(t)
	f.runs = []map[string]any{run(8, PhaseRunning)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	if _, err := c.Jobs.Wait(ctx, "shop", "migrate", 8, fast); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunAndWait(t *testing.T) {
	cases := []struct {
		name string
		last map[string]any
		exit int
	}{
		{"exits zero", run(8, PhaseDone, 0), 0},
		{"exits with the script's code", run(8, PhaseFailed, 3), 3},
		{"stopped without a code", run(8, PhaseFailed), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, c := newFake(t)
			f.runs = []map[string]any{run(7, PhaseDone, 0), run(8, PhaseRunning), tc.last}
			res, err := c.Jobs.RunAndWait(context.Background(), "shop", "migrate", RunOptions{Image: "m:1"}, fast)
			if err != nil {
				t.Fatal(err)
			}
			if res.Submission.Revision != 8 || res.Run.Revision != 8 || res.ExitCode() != tc.exit {
				t.Fatalf("result = %+v / %+v, exit %d", res.Submission, res.Run, res.ExitCode())
			}
		})
	}
}

func TestRunAndWaitRefusesASchedule(t *testing.T) {
	f, c := newFake(t)
	if _, err := c.Jobs.RunAndWait(context.Background(), "shop", "migrate", RunOptions{Image: "m:1", Schedule: "@daily"}); err == nil {
		t.Fatal("expected an error")
	}
	if len(f.requests) != 0 {
		t.Fatal("nothing should have been submitted")
	}
}

func TestRunAndWaitReturnsTheRefusal(t *testing.T) {
	_, c := newFake(t)
	res, err := c.Jobs.RunAndWait(context.Background(), "shop", "migrate", RunOptions{Image: "m:1", Size: "xl"}, fast)
	if !IsCode(err, "invalid_size") || res != nil {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestJobLogs(t *testing.T) {
	f, c := newFake(t)
	l, err := c.Jobs.Logs(context.Background(), "shop", "migrate", LogsOptions{Contains: "error"})
	if err != nil || l.Logs == "" || f.last().Query["q"][0] != "error" {
		t.Fatalf("logs = %+v, %v", l, err)
	}
}

func TestExitCodeOfNothing(t *testing.T) {
	var r *RunResult
	if r.ExitCode() != 1 {
		t.Fatal("a missing result should exit 1")
	}
}
