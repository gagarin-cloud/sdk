package gagarin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ProjectsService lists, creates and reads projects.
type ProjectsService struct{ c *Client }

// Project is a project as the API answers it.
type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	// Role is the caller's: "owner", "editor" or "viewer".
	Role               string     `json:"role"`
	SuspendedAt        *time.Time `json:"suspended_at,omitempty"`
	SuspendedReason    string     `json:"suspended_reason,omitempty"`
	AccountSuspendedAt *time.Time `json:"account_suspended_at,omitempty"`
}

// Suspended reports whether the project or its owner's account is
// suspended, which stops everything in it.
func (p *Project) Suspended() bool { return p.SuspendedAt != nil || p.AccountSuspendedAt != nil }

// List answers every project the caller can see, their own and shared.
func (s *ProjectsService) List(ctx context.Context) ([]Project, error) {
	var out struct {
		Projects []Project `json:"projects"`
	}
	if err := s.c.do(ctx, http.MethodGet, "/v1/projects", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Projects, nil
}

// Get resolves a project name or id.
func (s *ProjectsService) Get(ctx context.Context, project string) (*Project, error) {
	var out Project
	if err := s.c.do(ctx, http.MethodGet, projectPath(project), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create makes a project. Names are 2-30 lowercase letters, digits and
// hyphens, starting with a letter, and unique per account; a clash is
// refused "project_exists".
func (s *ProjectsService) Create(ctx context.Context, name string) (*Project, error) {
	var out Project
	if err := s.c.do(ctx, http.MethodPost, "/v1/projects", nil, map[string]string{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ProjectStatus is what a project is asked to run, next to what is actually
// running.
type ProjectStatus struct {
	Project   string          `json:"project"`
	ProjectID string          `json:"project_id"`
	Services  []ServiceStatus `json:"services"`
	Platform  PlatformState   `json:"platform"`
	// Notices are sentences a reader should be told before the table: a
	// suspension, a platform that is behind. Empty when there is nothing.
	Notices    []string   `json:"notices"`
	UsageToday UsageToday `json:"usage_today"`
}

// Service returns the named row, or nil.
func (p *ProjectStatus) Service(name string) *ServiceStatus {
	for i := range p.Services {
		if p.Services[i].Name == name {
			return &p.Services[i]
		}
	}
	return nil
}

// Service is a service, job or resource as stored: what it was asked to be.
// A job's TTLSeconds is zero when it is the default, MaxTTLSeconds.
type Service struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	// Kind is "container" for a service, "job" for a job, or a resource's
	// type ("postgres", "redis", ...).
	Kind           string    `json:"kind"`
	Size           string    `json:"size,omitempty"`
	Image          string    `json:"image"`
	Digest         string    `json:"digest,omitempty"`
	Port           int       `json:"port"`
	VolumeSizeGB   int       `json:"volume_size_gb,omitempty"`
	TimeoutSeconds int       `json:"timeout_seconds,omitempty"`
	TTLSeconds     int       `json:"ttl_seconds,omitempty"`
	Schedule       string    `json:"schedule,omitempty"`
	TimeZone       string    `json:"time_zone,omitempty"`
	CustomDomains  []string  `json:"custom_domains,omitempty"`
	UpdatedBy      string    `json:"updated_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// IsJob reports whether this is a job rather than a service or a resource.
func (s *Service) IsJob() bool { return s.Kind == "job" }

// ServiceStatus is one row of a project's status.
type ServiceStatus struct {
	Service
	Actual Actual `json:"actual"`
	// Needs is what this service may reach.
	Needs []string `json:"needs"`
	// InSync is whether the cluster matches what was asked for.
	InSync bool `json:"in_sync"`
	// Domains is every address it answers on, custom first, generated last.
	Domains []Domain `json:"domains"`
	// Sentence describes the row the way a person would.
	Sentence string `json:"sentence"`
	// EnvKeys is the names of its environment variables, never the values.
	EnvKeys        []string     `json:"env_keys"`
	VolumeUsage    *VolumeUsage `json:"volume_usage,omitempty"`
	ObjectUsage    *ObjectUsage `json:"object_usage,omitempty"`
	BackupsEnabled bool         `json:"backups_enabled"`
	// Restore is a restore under way into this resource, if any.
	Restore json.RawMessage `json:"restore,omitempty"`
}

// Actual is what the cluster is really running.
type Actual struct {
	Exists bool `json:"exists"`
	// ReadyReplicas counts pods of the asked-for revision that are serving.
	ReadyReplicas      int32  `json:"ready_replicas"`
	DesiredReplicas    int32  `json:"desired_replicas"`
	SupersededReplicas int32  `json:"superseded_replicas,omitempty"`
	RunningImage       string `json:"running_image,omitempty"`
	RunningPort        int32  `json:"running_port,omitempty"`
	IngressExists      bool   `json:"ingress_exists"`
	Message            string `json:"message,omitempty"`
	// Stalled is the cluster's verdict that a rollout stopped making progress.
	Stalled bool `json:"stalled,omitempty"`
	// Run is a job's latest run; nil for anything else, and before the first.
	Run *RunState `json:"run,omitempty"`
	// Schedule is a scheduled job's schedule; nil for anything else.
	Schedule *ScheduleState `json:"schedule,omitempty"`
}

// Domain is one address a service answers on.
type Domain struct {
	URL       string `json:"url"`
	Domain    string `json:"domain"`
	Generated bool   `json:"generated"`
	// State is how far along the address is, e.g. "live"; BlockedOn says on
	// whom, when it is not.
	State     string            `json:"state"`
	BlockedOn string            `json:"blocked_on,omitempty"`
	Sentence  string            `json:"sentence"`
	DNS       map[string]string `json:"dns,omitempty"`
}

// VolumeUsage is how full a volume is.
type VolumeUsage struct {
	UsedBytes     int64 `json:"used_bytes"`
	CapacityBytes int64 `json:"capacity_bytes"`
}

// ObjectUsage is what a bucket held at its last measurement.
type ObjectUsage struct {
	UsedBytes  int64     `json:"used_bytes"`
	MeasuredAt time.Time `json:"measured_at"`
}

// PlatformState is whether gagarin is converging this project on what it
// was asked for.
type PlatformState struct {
	ReconcilerLastRun *time.Time `json:"reconciler_last_run,omitempty"`
	Converging        bool       `json:"converging"`
}

// UsageToday is the project's accrued cost so far today (UTC).
type UsageToday struct {
	Services []ServiceUsage `json:"services"`
	Memory   struct {
		Memories int   `json:"memories"`
		MicroUSD int64 `json:"micro_usd"`
	} `json:"memory"`
	MicroUSD int64 `json:"micro_usd"`
}

// ServiceUsage is one service's metered time and cost today.
type ServiceUsage struct {
	Name     string `json:"name"`
	Minutes  int64  `json:"minutes"`
	MicroUSD int64  `json:"micro_usd"`
}

// Status reads what the project is asked to run and what is running.
func (s *ProjectsService) Status(ctx context.Context, project string) (*ProjectStatus, error) {
	var out ProjectStatus
	if err := s.c.do(ctx, http.MethodGet, projectPath(project)+"/status", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ServiceStatus reads one row of the project's status, refused "not_found"
// when there is no such name.
func (s *ProjectsService) ServiceStatus(ctx context.Context, project, name string) (*ServiceStatus, error) {
	st, err := s.Status(ctx, project)
	if err != nil {
		return nil, err
	}
	if row := st.Service(name); row != nil {
		return row, nil
	}
	return nil, &Error{StatusCode: http.StatusNotFound, Code: "not_found",
		Message: fmt.Sprintf("nothing named %s in project %s", name, project)}
}
