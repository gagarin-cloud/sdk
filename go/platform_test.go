package gagarin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthy(t *testing.T) {
	f, c := newFake(t)
	h, err := c.Platform.Health(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.last().Path != "/healthz/platform" || !h.OK || h.Cluster == nil || !h.Cluster.OK {
		t.Fatalf("health = %+v", h)
	}
}

func TestUnhealthyIsAnAnswerNotAnError(t *testing.T) {
	f, c := newFake(t)
	f.healthDown = true
	h, err := c.Platform.Health(context.Background())
	if err != nil {
		t.Fatalf("a 503 with a health body should be an answer: %v", err)
	}
	if h.OK || h.Reconciler == nil || h.Reconciler.OK || h.Reconciler.Detail == "" {
		t.Fatalf("health = %+v", h)
	}
}

func TestHealthFromSomethingThatIsNotGagarin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream connect error", http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := NewClient(WithToken("tok"), WithBaseURL(srv.URL))
	if _, err := c.Platform.Health(context.Background()); !IsCode(err, "http_502") {
		t.Fatalf("err = %v", err)
	}
}
