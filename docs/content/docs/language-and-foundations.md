---
title: Language and foundations
description: The Go implementation, protocol contracts, package boundaries, and documentation toolchain.
---

GOLFS's server is written in Go. `go.mod`, the Docker builder, and CI currently select Go 1.27.0. The executable is `cmd/golfs`; the runtime uses the standard library's HTTP server, contexts, structured JSON logging, and signal handling. The production Docker build disables CGO and embeds the application version and commit through linker flags.

## Protocol and provider foundations

The wire contract is Git LFS Batch and Verify with Basic Transfer and SHA-256 object identifiers. GitHub.com supplies repository identity and user permissions; the AWS SDK for Go v2 supplies S3 requests and presigning for Hetzner Object Storage. JWT signing uses `golang-jwt/jwt/v5`, and Prometheus client libraries expose operations metrics.

The separation is expressed through small Go interfaces: `forge.Authorizer` resolves a token and repository path into canonical identity and permission; `storage.ObjectStore` checks metadata, presigns transfers, and validates bucket access. The LFS handler consumes these interfaces without GitHub REST or AWS SDK types. Provider errors cross these boundaries as domain errors and become protocol responses.

This separation makes fake and local-server tests possible. It does not establish support for another forge or S3 provider: GitHub.com and the validated Hetzner upload contract remain the implemented deployment scope. See [Architecture](/architecture) for request flows and [Security](/security) for provider trust assumptions.

## Reading the implementation

All paths below are relative to the repository root:

| Path                                              | Responsibility                                                                            |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `cmd/golfs/main.go`                               | Wires adapters, validates startup, serves both listeners, and drains them on shutdown     |
| `internal/config`                                 | Loads environment settings and defines runtime defaults and hard limits                   |
| `internal/lfs`                                    | Implements Batch, Verify, request validation, authentication, and HTTP error mapping      |
| `internal/forge` and `internal/forge/github`      | Define the authorization contract and implement GitHub App checks                         |
| `internal/storage` and `internal/storage/s3store` | Define object actions and implement sharded keys, metadata checks, and signed S3 requests |
| `internal/authcache`                              | Bounds positive authorization caching within each process                                 |
| `internal/observability`                          | Defines metrics without repository or object labels                                       |

Start at `main.go` to see composition, then read the interfaces before their adapters. [Testing](/testing) identifies the automated coverage and remaining live validation gates.

## Deployment and documentation tooling

The Dockerfile produces a non-root Debian-based container with CA certificates. Helm templates wire configuration, Secrets, the two Services, and optional cluster integrations; they do not provision GitHub or object storage. [Installation](/installation) covers those dependencies.

The website is a separate TypeScript/React Next.js application using the Fumadocs MDX Macro API, Core, and Base UI. It exports static HTML, search data, Open Graph images, and processed Markdown. Node is pinned by `.nvmrc`, npm manages the `docs` workspace from the repository root, and authored pages live in `docs/content/docs`. The [docs README](https://github.com/kode-blox/golfs/blob/main/docs/README.md) describes the local authoring commands and generated endpoints. This tooling is separate from the Go server runtime.
