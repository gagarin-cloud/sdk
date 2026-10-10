package gagarin

import (
	"context"
	"testing"
)

func TestLogsSendsEveryFilter(t *testing.T) {
	f, c := newFake(t)
	l, err := c.Services.Logs(context.Background(), "shop", "migrate", LogsOptions{
		Limit: 50, Since: "6h", Until: "2026-10-10T09:00:00Z", Contains: "error", Previous: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	q := f.last().Query
	want := map[string]string{"limit": "50", "since": "6h", "until": "2026-10-10T09:00:00Z", "q": "error", "previous": "true"}
	for k, v := range want {
		if got := q[k]; len(got) != 1 || got[0] != v {
			t.Errorf("query %s = %v, want %s", k, got, v)
		}
	}
	if l.Source != "store" || l.Next == nil || *l.Next != "2026-10-10T09:00:00Z" || l.Logs == "" {
		t.Fatalf("logs = %+v", l)
	}
}

func TestLogsWithNoFiltersSendsNoQuery(t *testing.T) {
	f, c := newFake(t)
	if _, err := c.Services.Logs(context.Background(), "shop", "migrate", LogsOptions{}); err != nil {
		t.Fatal(err)
	}
	if q := f.last().Query; len(q) != 0 {
		t.Fatalf("query = %v", q)
	}
}

func TestDeployments(t *testing.T) {
	f, c := newFake(t)
	ds, err := c.Services.Deployments(context.Background(), "shop", "migrate")
	if err != nil {
		t.Fatal(err)
	}
	if got := f.last().Path; got != "/v1/projects/shop/services/migrate/deployments" {
		t.Fatalf("path = %s", got)
	}
	if len(ds) != 2 || ds[0].Revision != 7 || ds[0].EnvKeys[0] != "DATABASE_URL" || ds[1].RestoredFrom == nil || *ds[1].RestoredFrom != 4 {
		t.Fatalf("deployments = %+v", ds)
	}
}
