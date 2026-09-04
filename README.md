# GOLFS

GOLFS is a stateless Git LFS server for GitHub.com repositories and private S3-compatible object storage. It authenticates GitHub App users, authorizes repository access, implements the Git LFS Batch and Verify APIs, and returns direct presigned S3 actions. Object bytes never pass through GOLFS.

GOLFS v0.1 supports the Git LFS Basic Transfer protocol with SHA-256 objects up to 5,000,000,000 bytes. It deliberately excludes a database, multipart uploads, locking, garbage collection, object deletion, administration commands, and automatic fork provisioning.

## Request flow

1. A Git LFS client sends Basic credentials to the repository-specific Batch endpoint. The password is a GitHub App user access token.
2. GOLFS proves that the configured App is installed on the repository, that the user token can access that installation, and that the user has the needed repository permission.
3. GOLFS resolves GitHub's canonical numeric repository ID and checks S3 object metadata.
4. GOLFS returns a presigned S3 PUT or GET action. Uploads also receive a GOLFS Verify action.
5. S3 enforces the SHA-256 checksum, content length, and conditional object creation. GOLFS independently validates stored metadata during Verify and every download Batch.

Object keys have the stable form:

```text
github/{numeric-repository-id}/objects/{full-sha256-oid}
```

## API

The public listener serves:

```text
POST /github.com/{owner}/{repository}/info/lfs/objects/batch
POST /github.com/{owner}/{repository}/info/lfs/objects/verify
```

The cluster-internal operations listener serves:

```text
GET /healthz
GET /readyz
GET /metrics
```

There are no object proxy or redirect routes. See [Architecture](docs/architecture.md), [configuration](docs/configuration.md), and [operations](docs/operations.md) for the complete contract.

## Client setup

Create and install the GitHub App, then configure GCM 2.9.0 or later using the exact commands in [GitHub App and GCM setup](docs/github-app-and-gcm.md). Set the repository's LFS URL:

```shell
git config --local lfs.url "https://lfs.example.com/github.com/OWNER/REPOSITORY/info/lfs"
```

No GitHub App client secret is distributed.

## Development

Go 1.27.0 is required:

```shell
go vet ./...
go build ./cmd/golfs
```

Automated tests and provider-integration tooling are intentionally deferred during the initial implementation phase.

## Deployment

A non-root container and Helm chart are included. The chart exposes the public and operations listeners through separate Services and routes only the public Service through the optional Gateway API resources. See [the chart README](charts/README.md).

## License and provenance

GOLFS is an independent implementation licensed under Apache-2.0. Its protocol and architecture were informed by public Git LFS specifications and an existing Git LFS server, but its source and Git history are independent.
