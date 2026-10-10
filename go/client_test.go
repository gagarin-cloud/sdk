package gagarin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClientNeedsAToken(t *testing.T) {
	t.Setenv("GAGARIN_TOKEN", "")
	if _, err := NewClient(); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNewClientReadsTheEnvironment(t *testing.T) {
	f, _ := newFake(t)
	t.Setenv("GAGARIN_TOKEN", "tok")
	t.Setenv("GAGARIN_API", f.srv.URL)
	t.Setenv("GAGARIN_REGISTRY", "registry.from-env")
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Account.Whoami(context.Background()); err != nil {
		t.Fatalf("a client built from the environment did not reach the API: %v", err)
	}
	ref, err := c.ImageRef(context.Background(), "shop", "migrate:v3")
	if err != nil || ref != "registry.from-env/p-123/migrate:v3" {
		t.Fatalf("ImageRef = %q, %v", ref, err)
	}
}

func TestEveryRequestCarriesTheToken(t *testing.T) {
	f, c := newFake(t)
	c.Projects.List(context.Background())
	if got := f.last().Auth; got != "Bearer tok" {
		t.Fatalf("Authorization = %q", got)
	}
}

func TestARefusalIsATypedError(t *testing.T) {
	f, _ := newFake(t)
	c, _ := NewClient(WithToken("wrong"), WithBaseURL(f.srv.URL))
	_, err := c.Account.Whoami(context.Background())
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %#v, want *Error", err)
	}
	if e.StatusCode != http.StatusUnauthorized || e.Code != "unauthorized" || e.FixHint == "" {
		t.Fatalf("error = %+v", e)
	}
	if !IsCode(err, "unauthorized") || IsCode(err, "not_found") {
		t.Fatal("IsCode does not branch on the code")
	}
}

func TestANonEnvelopeFailureStillCarriesTheStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway from a proxy", http.StatusBadGateway)
	}))
	defer srv.Close()
	c, _ := NewClient(WithToken("tok"), WithBaseURL(srv.URL))
	_, err := c.Projects.List(context.Background())
	if !IsCode(err, "http_502") {
		t.Fatalf("err = %v", err)
	}
}

func TestImageRefLeavesAFullReferenceAlone(t *testing.T) {
	f, c := newFake(t)
	ref, err := c.ImageRef(context.Background(), "shop", "registry.gagarin.cloud/p-123/m:1")
	if err != nil || ref != "registry.gagarin.cloud/p-123/m:1" {
		t.Fatalf("ImageRef = %q, %v", ref, err)
	}
	if n := len(f.requests); n != 0 {
		t.Fatalf("a full reference made %d requests; it needs none", n)
	}
}

func TestImageRefOfAnUnknownProjectFails(t *testing.T) {
	_, c := newFake(t)
	if _, err := c.ImageRef(context.Background(), "nope", "m:1"); !IsCode(err, "not_found") {
		t.Fatalf("err = %v", err)
	}
}

type countingTransport struct {
	n         int
	userAgent string
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.n++
	c.userAgent = r.Header.Get("User-Agent")
	return http.DefaultTransport.RoundTrip(r)
}

func TestWithHTTPClient(t *testing.T) {
	f, _ := newFake(t)
	rt := &countingTransport{}
	c, _ := NewClient(WithToken("tok"), WithBaseURL(f.srv.URL), WithHTTPClient(&http.Client{Transport: rt}))
	if _, err := c.Projects.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.n != 1 || rt.userAgent != "gagarin-sdk-go" {
		t.Fatalf("transport saw %d requests, user agent %q", rt.n, rt.userAgent)
	}
}

func TestErrorTextCarriesCodeAndHint(t *testing.T) {
	e := &Error{Code: "invalid_size", Message: "too big", FixHint: "use s"}
	if got := e.Error(); got != "gagarin: [invalid_size] too big\nhint: use s" {
		t.Fatalf("Error() = %q", got)
	}
	if got := (&Error{Code: "x", Message: "y"}).Error(); got != "gagarin: [x] y" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestAnUnreachableAPIIsAnError(t *testing.T) {
	c, _ := NewClient(WithToken("tok"), WithBaseURL("http://127.0.0.1:1"))
	if _, err := c.Account.Billing(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := c.Platform.Health(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}
