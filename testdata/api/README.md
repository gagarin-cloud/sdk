# API fixtures

Response bodies shaped like the gagarin engine's own answers (field names and
nesting taken from `gagarin-cloud/core`, `engine/internal/api`). All three
SDKs' unit tests serve these from a local HTTP server, so a field renamed in a
fixture breaks every language at once.

They are illustrations, not recordings: no real account or project is in them.
The integration tests (`../../scripts/integration.sh`) check the same calls
against a real engine.
