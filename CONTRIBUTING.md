# Contributing

Use Go 1.27.2+ (the CI/build image is pinned to 1.27.2). Keep the project small and preserve configuration validation, private-chat authorization, verified TLS and desired/applied state semantics.

```sh
gofmt -w cmd internal migrations tests
go vet ./...
go test -race ./...
go build ./cmd/vpn-service
python3 scripts/fetch-singbox.py
SINGBOX_TEST_BINARY=/tmp/syranet-tools/sing-box go test -race -count=1 ./...
python3 scripts/e2e.py
```

The last command requires Docker Engine and creates only a randomly named disposable test project. It does not load your `.env` or use deployment volumes. Synthetic TLS material and random test credentials are generated per run. Database and Docker tests that skip without their explicit environment flags must not be reported as passing integration coverage.

Do not modify already applied migration files; add a new numbered SQL migration. Include regression tests for changed state transitions, authentication, URI/config compatibility and reconciliation. Explain incompatible deployment changes and migration procedures. Use example domains and documentation IP ranges; never include production credentials or client links in a PR, fixture or log.

For vulnerabilities, follow SECURITY.md. The project does not promise billing, TUN VPN clients, arbitrary mesh routing, device counting or zero-downtime configuration changes.
