package gagarin

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestIntegration drives a real gagarin API. It is skipped unless
// GAGARIN_INTEGRATION=1; see ../scripts/integration.sh for what it needs.
func TestIntegration(t *testing.T) {
	if os.Getenv("GAGARIN_INTEGRATION") != "1" {
		t.Skip("set GAGARIN_INTEGRATION=1 to run against a real API (scripts/integration.sh)")
	}
	project := mustEnv(t, "GAGARIN_IT_PROJECT")
	imageOK := mustEnv(t, "GAGARIN_IT_IMAGE_OK")
	imageFail := mustEnv(t, "GAGARIN_IT_IMAGE_FAIL")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	wait := WaitOptions{PollInterval: time.Second}

	t.Run("platform health", func(t *testing.T) {
		h, err := c.Platform.Health(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !h.Serving.OK {
			t.Fatalf("health = %+v", h)
		}
	})

	t.Run("whoami", func(t *testing.T) {
		w, err := c.Account.Whoami(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if w.Account == "" || !w.CanDeploy() || w.Registry == "" {
			t.Fatalf("whoami = %+v", w)
		}
	})

	t.Run("projects", func(t *testing.T) {
		if _, err := c.Projects.Create(ctx, project); err != nil && !IsCode(err, "project_exists") {
			t.Fatal(err)
		}
		if _, err := c.Projects.Create(ctx, project); !IsCode(err, "project_exists") {
			t.Fatalf("creating it again: %v", err)
		}
		p, err := c.Projects.Get(ctx, project)
		if err != nil || p.Name != project || p.ID == "" {
			t.Fatalf("project = %+v, %v", p, err)
		}
		byID, err := c.Projects.Get(ctx, p.ID)
		if err != nil || byID.Name != project {
			t.Fatalf("by id = %+v, %v", byID, err)
		}
		ps, err := c.Projects.List(ctx)
		if err != nil || !slices.ContainsFunc(ps, func(q Project) bool { return q.ID == p.ID }) {
			t.Fatalf("list = %+v, %v", ps, err)
		}
		if _, err := c.Projects.Get(ctx, "no-such-project-here"); err == nil {
			t.Fatal("an unknown project should be refused")
		}
	})

	t.Run("a job that succeeds", func(t *testing.T) {
		res, err := c.Jobs.RunAndWait(ctx, project, "it-go-ok", RunOptions{
			Image: imageOK, Env: map[string]string{"GREETING": "from-go"}, Size: SizeS, TimeoutSeconds: 300,
		}, wait)
		if err != nil {
			t.Fatal(err)
		}
		if res.ExitCode() != 0 || !res.Run.Succeeded() || res.Run.Revision != res.Submission.Revision {
			t.Fatalf("run = %+v", res.Run)
		}
		row, err := c.Jobs.Get(ctx, project, "it-go-ok")
		if err != nil || row.TimeoutSeconds != 300 || !slices.Contains(row.EnvKeys, "GREETING") {
			t.Fatalf("row = %+v, %v", row, err)
		}
		ds, err := c.Services.Deployments(ctx, project, "it-go-ok")
		if err != nil || len(ds) == 0 || ds[0].Revision != res.Submission.Revision {
			t.Fatalf("deployments = %+v, %v", ds, err)
		}
		logs := eventually(t, func() (string, bool) {
			l, err := c.Jobs.Logs(ctx, project, "it-go-ok", LogsOptions{})
			return logsText(l, err), err == nil && strings.Contains(logsText(l, err), "hello")
		})
		if !strings.Contains(logs, "hello") {
			t.Fatalf("logs = %q", logs)
		}
	})

	t.Run("a job that fails with its own code", func(t *testing.T) {
		res, err := c.Jobs.RunAndWait(ctx, project, "it-go-fail", RunOptions{Image: imageFail}, wait)
		if err != nil {
			t.Fatal(err)
		}
		if res.Run.Phase != PhaseFailed || res.ExitCode() != 3 {
			t.Fatalf("run = %+v, exit %d", res.Run, res.ExitCode())
		}
	})

	t.Run("status reports the jobs", func(t *testing.T) {
		st, err := c.Projects.Status(ctx, project)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"it-go-ok", "it-go-fail"} {
			row := st.Service(name)
			if row == nil || !row.IsJob() || row.Actual.Run == nil || !row.Actual.Run.Finished() {
				t.Fatalf("%s = %+v", name, row)
			}
		}
	})

	t.Run("refusals are typed", func(t *testing.T) {
		_, err := c.Jobs.Run(ctx, project, "it-go-ok", RunOptions{Image: imageOK, Size: "xl"})
		var e *Error
		if !errors.As(err, &e) || e.Code != "invalid_size" || e.StatusCode != 400 {
			t.Fatalf("err = %v", err)
		}
		if _, err := c.Jobs.Run(ctx, project, "it-go-ok", RunOptions{Image: imageOK, TimeoutSeconds: 99999}); !IsCode(err, "invalid_timeout") {
			t.Fatalf("err = %v", err)
		}
	})
}

func mustEnv(t *testing.T, k string) string {
	t.Helper()
	v := os.Getenv(k)
	if v == "" {
		t.Fatalf("%s is not set", k)
	}
	return v
}

func logsText(l *Logs, err error) string {
	if err != nil || l == nil {
		return ""
	}
	return l.Logs
}

// eventually retries a read for a while: a finished run's lines can reach
// the log store a moment after the run is reported finished.
func eventually(t *testing.T, f func() (string, bool)) string {
	t.Helper()
	var last string
	for i := 0; i < 30; i++ {
		v, ok := f()
		last = v
		if ok {
			return v
		}
		time.Sleep(time.Second)
	}
	return last
}
