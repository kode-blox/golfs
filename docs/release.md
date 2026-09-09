# Release process

The root `VERSION` is the authoritative application version. `charts/VERSION`
is the independent chart-version authority. `charts/Chart.yaml.appVersion` must
match the application version, while `charts/Chart.yaml.version` must match the
chart version.

Use the manually dispatched **Bump Version** workflow for routine releases. An
application release bumps both authorities so the deployable chart references
the new application. A chart-only release bumps only the chart authority. The
workflow uses a GitHub App installation token because pushes made with the
repository `GITHUB_TOKEN` do not trigger the downstream `push` workflow.

## Current workflow

`pipeline.yml` owns the external triggers, event conditions, reusable-workflow
ordering, permissions, and concurrency. The called workflows remain separate:

- `ci.yml` performs deterministic Go, Helm, vulnerability, lint, and GitHub
  Actions validation.
- `validate-container.yml` performs the pull-request-only disposable container
  build with registry push disabled.
- `release.yml` preserves the currently functional application and chart
  release process.

Pull requests run:

```text
pipeline -> ci -> validate-container(push=false)
```

A push to `main` runs CI and release eligibility checks. If either authoritative
version has no corresponding immutable Git tag, the pipeline calls `release.yml`.
If `VERSION` is untagged, Release builds and publishes the Linux amd64 image as
`ghcr.io/kode-blox/golfs:X.Y.Z`. If `charts/VERSION` is untagged, it publishes
the OCI chart as `ghcr.io/kode-blox/charts/golfs:X.Y.Z` and updates the selected
GOLFS dependency in the private GitOps charts repository. Chart-only releases
do not build an application container.

Successful releases create immutable `vX.Y.Z` application Git tags and
`chart-vX.Y.Z` chart Git tags. These Git tags record a validated release; they
are not version authority. No mutable container `latest` tag is created. After
the matching Git tag exists, Release creates an independent published GitHub
Release for each released authority.

The pipeline concurrency group is PR-specific for pull requests and branch-based
for `main`. Newer runs cancel older runs for the same pull request. Main runs do
not cancel an in-progress delivery; GitHub retains at most one pending run for
the branch, so main publication and deployment remain serialized.

## Build-once migration boundary

The published shared actions now provide the mechanics needed for the future
build-once path:

- `container-build-push@v1` publishes one tag and returns its image reference
  and digest.
- `container-promote@v1` creates one target tag from a supplied source digest
  without rebuilding the image.

GOLFS does not partially enable that path. The current release workflow remains
functional until the deployment prerequisites below exist and the complete
application transition can be made atomically:

```text
publish build-<full SHA>
  -> deploy dev with build-<full SHA>
  -> verify dev health
  -> promote the published digest to v<VERSION>
  -> deploy production with v<VERSION>
  -> verify production health
  -> create the application release record
```

Deployment remains tag-first. The digest is passed only from publication to
promotion to identify the exact image being retagged. Deployment verification
is a normal health and rollout check; it does not reproduce registry tag
immutability checks. Registry-side policy owns tag immutability.

The atomic migration must also update the production image-tag contract. The
current chart defaults the container tag to the unprefixed `.Chart.AppVersion`,
and the production wrapper does not override `image.tag`. Before promotion can
create only `v<VERSION>`, either the chart default or the reviewed GitOps update
contract must make production request that exact prefixed tag. Changing only
one side would make production reference a nonexistent image.

## Missing deployment prerequisites

Do not wire the build-once application path until all of these exist:

1. A real GOLFS dev wrapper and ApplicationSet target. The current GitOps setup
   contains only `golfs/envs/hetzner-fsn1-dc4-prod`, selected by the
   `hetznerFsn1Dc4Prod` cluster label.
2. A focused, reviewed deployment contract that updates `golfs.image.tag`
   without changing the independent chart version. It must accept
   `build-<full SHA>` for dev and `v<VERSION>` for production.
3. Dev credentials, a GitHub Environment selected by `pipeline.yml`, and either
   a routable health endpoint or runner access to the cluster-internal
   operations Service.
4. Deterministic deployment synchronization followed by ordinary dev and
   production health verification. `/healthz` and `/readyz` currently remain
   cluster-internal and are not exposed through the public Gateway.
5. The production GitHub App credentials and any required Environment approval
   protection.

## Required repository configuration

Create a GitHub App with repository `contents: write` permission and install it
for both `kode-blox/golfs` and
`SayakMukhopadhyay/k8s-landscape-charts`. Configure these values in GOLFS:

- Repository variable `GITOPS_APP_ID` containing the App ID.
- Repository secret `GITOPS_APP_PRIVATE_KEY` containing the App private key.
- Environment variable `GITOPS_ENVIRONMENT` containing the exact GitOps wrapper
  name, currently `hetzner-fsn1-dc4-prod`.
- Environment secret `OPENAI_API_KEY`, used only by `create-release` to write
  the descriptive portion of GitHub Release notes.
- GitHub Environment `production`. Configure required reviewers when a manual
  production gate is required.
- Environment variable `URL` containing the deployed GOLFS URL, shown on the
  production deployment record.

The production wrapper is
`golfs/envs/hetzner-fsn1-dc4-prod` in
`SayakMukhopadhyay/k8s-landscape-charts`. It must contain exactly one dependency
named `golfs` and commit its `Chart.lock` and vendored dependency archive. The
Release workflow updates only that dependency; it does not modify ApplicationSets,
bootstrap manifests, or the live cluster directly.

A fine-grained PAT restricted to the target repositories and `contents: write`
is an emergency fallback only. The checked-in workflows use the GitHub App path
and must not be changed to a PAT without review.

## GitHub Release notes

`create-release` owns release-note mechanics. It validates the already-pushed
immutable Git tag, derives the previous tag only from the same tag family, and
constructs the exact commit list and comparison/source links from Git. It uses
the OpenAI key only to generate a bounded descriptive summary and highlights;
an unavailable or malformed AI response fails before a GitHub Release is
created. Re-running after a partial failure is safe: existing tags must resolve
to the same commit and an existing matching GitHub Release is returned
unchanged. `release-tags` owns immutable Git-tag verification and atomic
creation; GOLFS selects the application and chart tag names and release order.
