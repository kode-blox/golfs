---
title: Testing
description: Automated protocol and adapter checks, documentation validation, and manual live-service gates.
---

GOLFS has automated Go tests and build checks, plus manual external-service acceptance gates. A passing local suite does not prove that Hetzner enforces signed uploads or that GitHub and GCM complete device login and token refresh.

## Local Go checks

Use Go 1.27.0 and run from the repository root, as in the [development quick start](https://github.com/kode-blox/golfs/blob/main/README.md#development):

```shell
go test ./...
go vet ./...
go build ./cmd/golfs
```

These commands do not start the service or require real GitHub/S3 credentials. Current test coverage is concentrated in four files:

| Test file                                  | What it checks                                                                                                                                |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/config/config_test.go`           | Installation-ID allowlist parsing, including missing, malformed, nonpositive, and duplicate entries                                           |
| `internal/forge/github/github_test.go`     | Allowlisted authorization, early denial of other installations, and startup checks for all configured installations using a local HTTP server |
| `internal/lfs/handler_test.go`             | Upload action selection, existing-object size conflicts, Verify outcomes, and download size checks using fake providers                       |
| `internal/storage/s3store/s3store_test.go` | Sharded keys, OID validation, signed upload headers, path and expiry, and HEAD size checks using local presigning and a test HTTP server      |

The tests do not provide complete coverage of every runtime limit, cache behavior, listener lifecycle, or error path. They do not exercise a live provider, GCM, or Kubernetes.

## CI and documentation checks

The [CI workflow](https://github.com/kode-blox/golfs/blob/main/.github/workflows/ci.yaml) also verifies module metadata with `go mod tidy` and a clean module diff, checks `gofmt`, runs golangci-lint and actionlint, and builds the executable. CodeQL analyzes Go; dependency review applies on pull requests. Helm validation packages the chart without publishing and renders both defaults and a combination of optional Gateway, External Secrets, namespace, ServiceMonitor, autoscaling, and ServiceAccount resources. Rendering does not prove that controllers or CRDs exist in a target cluster.

Pull requests and manual runs of the main pipeline also select disposable container validation with publishing disabled. See [Release process](/release-process) for the trigger and delivery graph.

For website changes, use the Node version in `.nvmrc` and the root npm workspace. After installing dependencies as described in the [docs README](https://github.com/kode-blox/golfs/blob/main/docs/README.md), run:

```shell
npm run lint --workspace @golfs/docs
npm run types:check --workspace @golfs/docs
npm run build --workspace @golfs/docs
```

The documentation workflow runs those checks and validates the generated static site. When changing page names, navigation, or links, inspect the exported HTML, search URLs, `llms.txt`, `llms-full.txt`, and individual Markdown routes in `docs/out`. The build does not validate the behavior of linked external services.

## Manual external-service gates

Use a disposable test repository and test storage objects. Run these gates before relying on a new deployment and repeat affected gates after material changes to authentication, signing, the provider, or client tooling. Startup's App and bucket-access checks are necessary but do not replace these tests.

1. Complete the [required GitHub App/GCM live gate](/github-app-and-gcm#required-live-gate): device authorization without a client secret, acceptance only for the configured App and allowlisted installation, token refresh, and rejection of disallowed credentials and installations.
2. Upload a new valid LFS object through the real Batch/PUT/Verify flow. Download it from a fresh clone and confirm the object bytes hash to the OID. Run `git lfs fsck --objects` and `git lfs fsck --pointers` in the test repository.
3. For a disposable absent object, submit a PUT with bytes that do not match the signed `x-amz-content-sha256` header while retaining the signed size and required headers. Confirm Hetzner rejects it and leaves no accepted object.
4. Exercise conditional creation against a disposable existing key using a still-valid signed PUT with `If-None-Match: *`. Confirm a second write cannot replace the stored object. Re-Batch and Verify the accepted object to check retry convergence.
5. Confirm HTTPS client access and that health, readiness, and metrics remain reachable only through the internal operations Service. Readiness alone is not evidence of upload integrity or client authentication.

Keep tokens, signed URLs, private keys, and test object contents out of public logs and issues. Record gate outcomes for the selected deployment and tool versions. [Security](/security) explains why provider enforcement and trusted operator writes are part of the integrity contract; another S3 provider requires a new conformance decision.
