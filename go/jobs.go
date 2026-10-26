package gagarin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
//
// SizeG is the GPU size: one L4 GPU (24 GB) with 6 vCPU and 40 GiB, dedicated,
// and for jobs only. The API accepts it on a run and refuses it on a service
// deploy or a resource with invalid_size. It costs $3 an hour, by the minute,
// and a run sits pending for three to six minutes while the GPU node boots.
const (
	SizeS = "s"
	SizeM = "m"
	SizeL = "l"
	SizeG = "g"
)

// MaxTimeoutSeconds is the longest a run may take, and what a new job gets.
const MaxTimeoutSeconds = 3600

// MaxTTLSeconds is the longest a finished run is kept, and what a new job gets.
const MaxTTLSeconds = 28800

// MinTTLSeconds is the shortest a finished run may be kept: long enough that
// whoever waits on it reads how it ended before it goes.
const MinTTLSeconds = 60

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
	// Size is SizeS, SizeM, SizeL or SizeG (a GPU: $3 an hour, by the minute,
	// jobs only). Empty keeps the job's current size.
	Size string
	// TimeoutSeconds stops the run after this long, 1 to MaxTimeoutSeconds,
	// counted from when the run gets a machine, so pulling the image uses some
	// of it and waiting for a machine does not. Zero keeps the job's current
	// timeout.
	TimeoutSeconds int
	// TTLSeconds is how long a finished run is kept, MinTTLSeconds to MaxTTLSeconds,
	// counted from when the run ended. Then a one-shot job is deleted
	// entirely, history and all; a scheduled job stays and loses only that
	// firing. Zero keeps the job's current TTL.
	TTLSeconds int
	// Schedule makes the job a scheduled one: five cron fields ("0 3 * * *")
	// or a descriptor (@daily). Nothing runs now; each firing is a run. Empty
	// keeps the current schedule. A schedule cannot be removed except by
	// destroying the job.
	Schedule string
	// TimeZone is the IANA zone the schedule is read in. Empty keeps the
	// current one (UTC for a new job).
	TimeZone string
}

// Submission is what submitting a run answers: the job as stored, and the
// revision this run is. The revision is the run's name.
type Submission struct {
	Service
	Revision int `json:"revision"`
}

// RunState is what a run did.
type RunState struct {
	Revision   int        `json:"revision"`
	Phase      string     `json:"phase"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// Expired is set on a scheduled firing whose TTL ran out: it is no longer
	// in the cluster, and this is the platform's record of it.
	Expired bool `json:"expired,omitempty"`
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
	if opts.TTLSeconds != 0 {
		body["ttl_seconds"] = opts.TTLSeconds
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

// Get reads the named job's row from the project's status, with how its
// latest run went in Actual.Run. A name that is not a job is refused
// "not_a_job".
func (j *JobsService) Get(ctx context.Context, project, name string) (*ServiceStatus, error) {
	row, err := j.c.Projects.ServiceStatus(ctx, project, name)
	if err != nil {
		return nil, err
	}
	if !row.IsJob() {
		return nil, &Error{StatusCode: http.StatusConflict, Code: "not_a_job",
			Message: fmt.Sprintf("%s is a %s in this project, not a job", name, row.Kind)}
	}
	return row, nil
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

// Logs reads the job's logs. Every run's lines are there, newest last.
func (j *JobsService) Logs(ctx context.Context, project, name string, opts LogsOptions) (*Logs, error) {
	return j.c.Services.Logs(ctx, project, name, opts)
}
