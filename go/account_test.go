package gagarin

import (
	"context"
	"testing"
)

func TestWhoami(t *testing.T) {
	f, c := newFake(t)
	w, err := c.Account.Whoami(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r := f.last(); r.Method != "GET" || r.Path != "/v1/whoami" {
		t.Fatalf("request = %s %s", r.Method, r.Path)
	}
	if w.Account != "dev@example.com" || w.Registry != "registry.example" || w.Platform != "linux/amd64" ||
		w.Client != "gg on laptop" || w.AccountCreatedAt.IsZero() {
		t.Fatalf("whoami = %+v", w)
	}
	if !w.CanDeploy() || w.CanDestroy() {
		t.Fatalf("can = %v", w.Can)
	}
}

func TestBilling(t *testing.T) {
	f, c := newFake(t)
	b, err := c.Account.Billing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r := f.last(); r.Path != "/v1/billing" {
		t.Fatalf("path = %s", r.Path)
	}
	if b.MicroUSD != 12345678 || b.Balance != "$12.35" || b.BurnPerHourMicroUSD != 28000 ||
		b.RunwayHours == nil || *b.RunwayHours != 440.9 || !b.HasCard || b.Services != 2 {
		t.Fatalf("billing = %+v", b)
	}
}
