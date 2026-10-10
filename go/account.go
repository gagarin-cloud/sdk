package gagarin

import (
	"context"
	"net/http"
	"time"
)

// AccountService reads the account the credential belongs to.
type AccountService struct{ c *Client }

// Whoami is who the credential is, what it may do, and where gagarin puts
// things.
type Whoami struct {
	// Account is the account's email address.
	Account          string    `json:"account"`
	AccountCreatedAt time.Time `json:"account_created_at"`
	// Client is the name the credential was issued to, e.g. "gg on laptop".
	Client string `json:"client"`
	// Can is the credential's capabilities: "deploy", "destroy", both or
	// neither.
	Can []string `json:"can"`
	// BaseDomain is what generated service addresses end in.
	BaseDomain string `json:"base_domain"`
	// Registry is the registry host images are run from.
	Registry string `json:"registry"`
	// Platform is the cluster's OS/architecture, e.g. "linux/amd64": what
	// an image must be built for.
	Platform string `json:"platform"`
}

// CanDeploy reports whether the credential may deploy and run.
func (w *Whoami) CanDeploy() bool { return w.has("deploy") }

// CanDestroy reports whether the credential may destroy without asking.
func (w *Whoami) CanDestroy() bool { return w.has("destroy") }

func (w *Whoami) has(scope string) bool {
	for _, s := range w.Can {
		if s == scope {
			return true
		}
	}
	return false
}

// Whoami says which account this credential acts as.
func (a *AccountService) Whoami(ctx context.Context) (*Whoami, error) {
	var out Whoami
	if err := a.c.do(ctx, http.MethodGet, "/v1/whoami", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Billing is where the account stands: its balance, what it is burning, and
// how long that lasts. Money is in micro-USD (1 USD = 1,000,000), with a
// formatted string beside the figures a person reads.
type Billing struct {
	MicroUSD            int64  `json:"micro_usd"`
	Balance             string `json:"balance"`
	BurnPerHourMicroUSD int64  `json:"burn_per_hour_micro_usd"`
	// RunwayHours is nil when nothing is burning: the balance lasts forever.
	RunwayHours      *float64 `json:"runway_hours"`
	HasCard          bool     `json:"has_card"`
	AutoTopUp        bool     `json:"auto_top_up"`
	Suspended        bool     `json:"suspended"`
	TopUpAt          string   `json:"top_up_at"`
	TopUpFor         string   `json:"top_up_for"`
	TopUpAtMicroUSD  int64    `json:"top_up_at_micro_usd"`
	TopUpForMicroUSD int64    `json:"top_up_for_micro_usd"`
	// Services is how many services are on the meter now.
	Services int `json:"services"`
	Memories int `json:"memories"`
}

// Billing reads the account's balance and burn.
func (a *AccountService) Billing(ctx context.Context) (*Billing, error) {
	var out Billing
	if err := a.c.do(ctx, http.MethodGet, "/v1/billing", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
