package gagarin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Run phases, as the API reports them. Pending and running mean wait; done and
// failed mean read the exit code; suspended means the project was suspended.
const (
	PhasePending   = "pending"
	PhaseRunning   = "running"
	PhaseDone      = "done"
	PhaseFailed    = "failed"
	PhaseSuspended = "suspended"
)

// Sizes a job can be given. Billed for the time it runs, at its size.
const (
	SizeS = "s"
	SizeM = "m"
	SizeL = "l"
)

// MaxTimeoutSeconds is the longest a run may take, and what a new job gets.
const MaxTimeoutSeconds = 3600

// JobsService submits, waits on and reads jobs: images that run to completion
// and listen on nothing.
type JobsService struct{ c *Client }

// RunOptions is everything the API accepts for a job.
//
// As with `gg run`, the image must already be in gagarin's registry. Every call
// is a new run and a new revision.
type RunOptions struct {
	// Image is the full reference in gagarin's registry
	// ("registry.gagarin.cloud/<project-id>/migrate:v3"), or a bare
	// "migrate:v3" that the client expands against the project. Required.
	Image string
	// Digest pins the exact image: "sha256:..." as docker push reported it.
	Digest string
	// Env is the complete environment for this run. It replaces the job's
	// environment; it is not merged, and nil empties it.
	Env map[string]string
	// Deps are resources and services this run may reach, added to whatever
	// the job already declares. A migration must name its database here, or
	// its connection hangs rather than failing.
	Deps []string
	// Size is SizeS, SizeM or SizeL. Empty keeps the job's current size.
	Size string
	// TimeoutSeconds stops the run after this long, 1 to MaxTimeoutSeconds,
	// counted from submission. Zero keeps the job's current timeout.
	TimeoutSeconds int
	// Schedule makes the job a scheduled one: five cron fields ("0 3 * * *")
	// or a descriptor (@daily). Nothing runs now; each firing is a run. Empty
	// keeps the current schedule. A schedule cannot be removed except by
	// destroying the job.
	Schedule string
	// TimeZone is the IANA zone the schedule is read in. Empty keeps the
	// current one (UTC for a new job).
	TimeZone string
}

// Job is the job as stored after a submission.
type Job struct {
	Project        string    `json:"project"`
	Name           string    `json:"name"`
	Kind           string    `json:"kind"`
	Size           string    `json:"size,omitempty"`
	Image          string    `json:"image"`
	Digest         string    `json:"digest,omitempty"`
	TimeoutSeconds int       `json:"timeout_seconds,omitempty"`
	Schedule       string    `json:"schedule,omitempty"`
	TimeZone       string    `json:"time_zone,omitempty"`
	UpdatedBy      string    `json:"updated_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Submission is what submitting a run answers: the job as stored, and the
// revision this run is. The revision is the run's name.
type Submission struct {
	Job
	Revision int `json:"revision"`
}

// RunState is what a run did.
type RunState struct {
	Revision   int        `json:"revision"`
	Phase      string     `json:"phase"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Finished reports whether the run is over, either way.
func (r *RunState) Finished() bool {
	return r != nil && (r.Phase == PhaseDone || r.Phase == PhaseFailed)
}

// Succeeded reports whether the run finished and exited zero.
func (r *RunState) Succeeded() bool {
	return r != nil && r.Phase == PhaseDone && (r.ExitCode == nil || *r.ExitCode == 0)
}

// ScheduleState is what a scheduled job's schedule says.
type ScheduleState struct {
	NextAt          *time.Time `json:"next_at,omitempty"`
	LastScheduledAt *time.Time `json:"last_scheduled_at,omitempty"`
	Suspended       bool       `json:"suspended,omitempty"`
}

// JobStatus is one job's row from the project's status.
type JobStatus struct {
	Job
	Needs    []string `json:"needs"`
	InSync   bool     `json:"in_sync"`
	Sentence string   `json:"sentence"`
	EnvKeys  []string `json:"env_keys"`
	Actual   struct {
		Exists   bool           `json:"exists"`
		Message  string         `json:"message,omitempty"`
		Run      *RunState      `json:"run,omitempty"`
		Schedule *ScheduleState `json:"schedule,omitempty"`
	} `json:"actual"`
}

// Run submits a run of the named job and returns without waiting.
func (j *JobsService) Run(ctx context.Context, project, name string, opts RunOptions) (*Submission, error) {
	if opts.Image == "" {
		return nil, errors.New("gagarin: RunOptions.Image is required")
	}
	image, err := j.c.ImageRef(ctx, project, opts.Image)
	if err != nil {
		return nil, err
	}
	// The environment is replaced on every run, so it is always sent: an
	// empty one says what nil means rather than leaving it to the server.
	env := opts.Env
	if env == nil {
		env = map[string]string{}
	}
	body := map[string]any{"kind": "job", "image": image, "env": env}
	if opts.Digest != "" {
		body["digest"] = opts.Digest
	}
	if len(opts.Deps) > 0 {
		body["deps"] = opts.Deps
	}
	if opts.Size != "" {
		body["size"] = opts.Size
	}
	if opts.TimeoutSeconds != 0 {
		body["timeout_seconds"] = opts.TimeoutSeconds
	}
	if opts.Schedule != "" {
		body["schedule"] = opts.Schedule
	}
	if opts.TimeZone != "" {
		body["time_zone"] = opts.TimeZone
	}
	var out Submission
	if err := j.c.do(ctx, http.MethodPut, servicePath(project, name), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get reads the named job's status: its row and how its latest run went.
func (j *JobsService) Get(ctx context.Context, project, name string) (*JobStatus, error) {
	var st struct {
		Services []JobStatus `json:"services"`
	}
	if err := j.c.do(ctx, http.MethodGet, "/v1/projects/"+url.PathEscape(project)+"/status", nil, nil, &st); err != nil {
		return nil, err
	}
	for i := range st.Services {
		if st.Services[i].Name == name {
			if st.Services[i].Kind != "job" {
				return nil, &Error{StatusCode: http.StatusConflict, Code: "not_a_job",
					Message: fmt.Sprintf("%s is a %s in this project, not a job", name, st.Services[i].Kind)}
			}
			return &st.Services[i], nil
		}
	}
	return nil, &Error{StatusCode: http.StatusNotFound, Code: "not_found",
		Message: fmt.Sprintf("no job %s in project %s", name, project)}
}

// WaitOptions tunes Wait.
type WaitOptions struct {
	// PollInterval is how often status is read. Defaults to 3s.
	PollInterval time.Duration
	// Timeout bounds the wait when ctx has no deadline. Defaults to 65
	// minutes: a run is stopped by the platform at 60.
	Timeout time.Duration
	// OnUpdate, if set, is called each time the run's phase changes.
	OnUpdate func(*RunState)
}

// ErrSuperseded is returned by Wait when a newer run of the job started
// before the awaited one could be observed finishing.
var ErrSuperseded = errors.New("gagarin: the run was superseded by a newer one")

// ErrSuspended is returned by Wait when the project was suspended, which
// stops its runs.
var ErrSuspended = errors.New("gagarin: the project is suspended and its run was stopped")

// Wait blocks until the given revision of the job has finished, and returns
// how it ended. A run that failed is not an error: read Phase and ExitCode.
func (j *JobsService) Wait(ctx context.Context, project, name string, revision int, opts WaitOptions) (*RunState, error) {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 3 * time.Second
	}
	if _, ok := ctx.Deadline(); !ok {
		if opts.Timeout <= 0 {
			opts.Timeout = 65 * time.Minute
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	lastPhase, transient := "", 0
	for {
		st, err := j.Get(ctx, project, name)
		switch {
		case err == nil:
			transient = 0
			run := st.Actual.Run
			if run != nil && run.Revision > revision {
				return run, ErrSuperseded
			}
			if run != nil && run.Revision == revision {
				if run.Phase != lastPhase && opts.OnUpdate != nil {
					opts.OnUpdate(run)
				}
				lastPhase = run.Phase
				if run.Finished() {
					return run, nil
				}
				if run.Phase == PhaseSuspended {
					return run, ErrSuspended
				}
			}
		case retryable(err) && transient < 5:
			transient++
		default:
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("gagarin: waiting for run %d of %s: %w", revision, name, ctx.Err())
		case <-time.After(opts.PollInterval):
		}
	}
}

func retryable(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.StatusCode >= 500 || e.StatusCode == http.StatusTooManyRequests
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// RunResult is a submission and how its run ended.
type RunResult struct {
	Submission *Submission
	Run        *RunState
}

// ExitCode is the process's exit code the way `gg run` exits with it: the
// script's own code, 0 on success, and 1 for a run that failed without one
// (stopped by its timeout, say).
func (r *RunResult) ExitCode() int {
	if r == nil || r.Run == nil {
		return 1
	}
	if r.Run.ExitCode != nil {
		return *r.Run.ExitCode
	}
	if r.Run.Succeeded() {
		return 0
	}
	return 1
}

// RunAndWait submits a run and waits for it to finish, which is what `gg run`
// does. A scheduled job runs nothing now, so Schedule is refused here; submit
// it with Run.
func (j *JobsService) RunAndWait(ctx context.Context, project, name string, opts RunOptions, wait ...WaitOptions) (*RunResult, error) {
	if opts.Schedule != "" {
		return nil, errors.New("gagarin: a scheduled job does not run now; submit it with Run")
	}
	sub, err := j.Run(ctx, project, name, opts)
	if err != nil {
		return nil, err
	}
	var w WaitOptions
	if len(wait) > 0 {
		w = wait[0]
	}
	run, err := j.Wait(ctx, project, name, sub.Revision, w)
	return &RunResult{Submission: sub, Run: run}, err
}

// LogsOptions narrows a read of a job's logs. All optional.
type LogsOptions struct {
	// Limit is how many lines, newest last.
	Limit int
	// Since and Until are a duration back from now ("30m", "6h", "2d") or
	// an RFC 3339 time. Until is usually the Next of the previous page.
	Since, Until string
	// Contains keeps only lines containing this text.
	Contains string
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

// Logs reads the job's logs.
func (j *JobsService) Logs(ctx context.Context, project, name string, opts LogsOptions) (*Logs, error) {
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
	var out Logs
	if err := j.c.do(ctx, http.MethodGet, servicePath(project, name)+"/logs", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func servicePath(project, name string) string {
	return "/v1/projects/" + url.PathEscape(project) + "/services/" + url.PathEscape(name)
}
