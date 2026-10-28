package gagarin

import (
	"context"
	"testing"
)

func TestProjectsList(t *testing.T) {
	_, c := newFake(t)
	ps, err := c.Projects.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].ID != "p-123" || ps[0].Role != "owner" || ps[0].Cluster != "fr-par-1" || ps[0].Suspended() {
		t.Fatalf("projects = %+v", ps)
	}
	if !ps[1].Suspended() || ps[1].SuspendedReason == "" {
		t.Fatalf("a suspended project does not say so: %+v", ps[1])
	}
}

func TestProjectsGetByNameOrID(t *testing.T) {
	f, c := newFake(t)
	for _, ref := range []string{"shop", "p-123"} {
		p, err := c.Projects.Get(context.Background(), ref)
		if err != nil || p.ID != "p-123" || p.Name != "shop" {
			t.Fatalf("Get(%q) = %+v, %v", ref, p, err)
		}
		if got := f.last().Path; got != "/v1/projects/"+ref {
			t.Fatalf("path = %s", got)
		}
	}
	if _, err := c.Projects.Get(context.Background(), "nope"); !IsCode(err, "not_found") {
		t.Fatalf("err = %v", err)
	}
}

func TestProjectsGetEscapesTheName(t *testing.T) {
	f, c := newFake(t)
	c.Projects.Get(context.Background(), "a/b")
	// A slash in a name must not become a path segment.
	if got := f.last().Escaped; got != "/v1/projects/a%2Fb" {
		t.Fatalf("path = %s", got)
	}
}

func TestProjectsCreate(t *testing.T) {
	f, c := newFake(t)
	p, err := c.Projects.Create(context.Background(), "fresh")
	if err != nil {
		t.Fatal(err)
	}
	r := f.last()
	if r.Method != "POST" || r.Path != "/v1/projects" || r.ContentType != "application/json" || r.Body["name"] != "fresh" {
		t.Fatalf("request = %+v", r)
	}
	if p.ID != "p-789" || p.Name != "fresh" {
		t.Fatalf("project = %+v", p)
	}
	if _, err := c.Projects.Create(context.Background(), "shop"); !IsCode(err, "project_exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestProjectStatus(t *testing.T) {
	f, c := newFake(t)
	st, err := c.Projects.Status(context.Background(), "shop")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.last().Path; got != "/v1/projects/shop/status" {
		t.Fatalf("path = %s", got)
	}
	if st.ProjectID != "p-123" || st.Cluster != "fr-par-1" || len(st.Services) != 4 || !st.Platform.Converging || st.UsageToday.MicroUSD != 141467 {
		t.Fatalf("status = %+v", st)
	}

	web := st.Service("web")
	if web == nil || web.Kind != "container" || web.Port != 8080 || !web.InSync || web.Actual.ReadyReplicas != 1 ||
		len(web.Domains) != 2 || web.Domains[1].Generated != true || web.Needs[0] != "db" || len(web.EnvKeys) != 2 {
		t.Fatalf("web = %+v", web)
	}
	db := st.Service("db")
	if db == nil || db.VolumeUsage == nil || db.VolumeUsage.CapacityBytes != 10737418240 || !db.BackupsEnabled {
		t.Fatalf("db = %+v", db)
	}
	migrate := st.Service("migrate")
	if migrate == nil || !migrate.IsJob() || migrate.Actual.Run == nil || migrate.Actual.Run.Revision != 7 ||
		migrate.Actual.Run.Phase != PhaseFailed || *migrate.Actual.Run.ExitCode != 3 || migrate.TimeoutSeconds != 900 {
		t.Fatalf("migrate = %+v", migrate)
	}
	nightly := st.Service("nightly")
	if nightly == nil || nightly.Schedule != "0 3 * * *" || nightly.Actual.Schedule == nil || nightly.Actual.Schedule.NextAt == nil {
		t.Fatalf("nightly = %+v", nightly)
	}
	if st.Service("nope") != nil {
		t.Fatal("a missing name should be nil")
	}
}

func TestServiceStatusOfAMissingName(t *testing.T) {
	_, c := newFake(t)
	if _, err := c.Projects.ServiceStatus(context.Background(), "shop", "nope"); !IsCode(err, "not_found") {
		t.Fatalf("err = %v", err)
	}
}
