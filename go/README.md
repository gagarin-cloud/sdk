# Gagarin — Go

```sh
go get github.com/gagarin-cloud/sdk/go
```

```go
import gagarin "github.com/gagarin-cloud/sdk/go"

c, err := gagarin.NewClient() // GAGARIN_TOKEN, GAGARIN_API; or gagarin.WithToken(...)
if err != nil { log.Fatal(err) }

// What `gg run` does: submit, wait, exit with the script's code.
res, err := c.Jobs.RunAndWait(ctx, "shop", "migrate", gagarin.RunOptions{
	Image:          "migrate:v3", // or registry.gagarin.cloud/<project-id>/migrate:v3
	Env:            map[string]string{"LOG_LEVEL": "debug"},
	Deps:           []string{"db"},
	Size:           gagarin.SizeM,
	TimeoutSeconds: 900,
}, gagarin.WaitOptions{OnUpdate: func(r *gagarin.RunState) { log.Println(r.Phase) }})
if err != nil { log.Fatal(err) }
os.Exit(res.ExitCode())
```

Fire and forget, then check later:

```go
sub, err := c.Jobs.Run(ctx, "shop", "backfill", gagarin.RunOptions{Image: "backfill:v1"})
run, err := c.Jobs.Wait(ctx, "shop", "backfill", sub.Revision, gagarin.WaitOptions{})
logs, err := c.Jobs.Logs(ctx, "shop", "backfill", gagarin.LogsOptions{Since: "30m"})
```

A scheduled job (runs nothing now):

```go
c.Jobs.Run(ctx, "shop", "nightly", gagarin.RunOptions{
	Image: "report:v2", Schedule: "0 3 * * *", TimeZone: "Europe/Berlin",
})
```

The basics:

```go
me, _ := c.Account.Whoami(ctx)          // me.Account, me.CanDeploy(), me.Registry
bill, _ := c.Account.Billing(ctx)       // bill.Balance, bill.RunwayHours
projects, _ := c.Projects.List(ctx)
p, _ := c.Projects.Create(ctx, "shop")  // refused "project_exists" if you have one
st, _ := c.Projects.Status(ctx, "shop") // st.Services, st.Notices, st.UsageToday
web := st.Service("web")                // web.InSync, web.Actual.ReadyReplicas, web.Domains
logs, _ := c.Services.Logs(ctx, "shop", "web", gagarin.LogsOptions{Since: "1h", Contains: "error"})
history, _ := c.Services.Deployments(ctx, "shop", "web")
health, _ := c.Platform.Health(ctx)     // an unhealthy platform is an answer, not an error
```

API refusals are `*gagarin.Error`; use `gagarin.IsCode(err, "invalid_timeout")`.

```sh
go test ./...
```
