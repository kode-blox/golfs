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

A push to `main` runs the caller-owned CI/CD Pipeline workflow. Alongside CI,
the pipeline validates the two authoritative version files and checks whether
their corresponding immutable release tags exist. It calls the separate Release
workflow only when CI succeeds and the current application or chart version is
not already tagged. This lets a later passing `main` commit complete a release
that an earlier version-bump commit could not start because CI failed, without
requiring a synthetic version edit. If `VERSION` is untagged, Release publishes
the Linux amd64 image as `ghcr.io/kode-blox/golfs:X.Y.Z`. If `charts/VERSION` is
untagged, it publishes the OCI chart as `ghcr.io/kode-blox/charts/golfs:X.Y.Z`
and then updates the selected GOLFS dependency in the private GitOps charts
repository. Successful releases create immutable `vX.Y.Z` application tags and
`chart-vX.Y.Z` chart tags. Tags record a validated release; they are not version
authority. No mutable `latest` tag is created. After the matching tag exists,
Release creates an independent published GitHub Release for each released
authority. The application and chart therefore have separate release histories
even when both are released by the same push.

## Required repository configuration

Create a GitHub App with repository `contents: write` permission and install it
for both `kode-blox/golfs` and
`SayakMukhopadhyay/k8s-landscape-charts`. Configure these values in GOLFS:

- Repository variable `GITOPS_APP_ID` containing the App ID.
- Repository secret `GITOPS_APP_PRIVATE_KEY` containing the App private key.
- Environment variable `GITOPS_ENVIRONMENT` containing the GitOps deployment
  name, such as `production`.
- Repository or `production` secret `OPENAI_API_KEY`, used only
  by `create-release` to write the descriptive portion of GitHub Release notes.
- GitHub Environment `production` with required reviewers.
- Environment variable `URL` containing the deployed GOLFS URL, shown on the
  production deployment record.

The wrapper chart and matching ApplicationSet must exist before the first push
to `main`. Following the personal GitOps convention, the production wrapper is
`golfs/envs/production` in `SayakMukhopadhyay/k8s-landscape-charts`; the
reusable action defaults the target repository to that repository, the target
branch to `main`, the dependency to the chart name, and the wrapper location to
`<chart-name>/envs/<environment>`. The wrapper must contain exactly one
dependency named `golfs` and commit its `Chart.lock` and vendored dependency
archive. The Release workflow updates only that dependency; it does not modify
appsets, bootstrap manifests, or the live cluster directly.

A fine-grained PAT restricted to the target repositories and `contents: write`
is an emergency fallback only. The checked-in workflows are configured for the
GitHub App path and must not be changed to a PAT without review.

The initial push is also a release candidate because every file is new in that
push range. Confirm the GitOps wrappers, ApplicationSets, variables, secret, and
approval Environment before pushing it.

## GitHub Release notes

`create-release` owns release-note mechanics. It validates the already-pushed
immutable tag, derives the previous tag only from the same tag family, and
constructs the exact commit list and comparison/source links from Git. It uses
the OpenAI key only to generate a bounded descriptive summary and highlights;
an unavailable or malformed AI response fails before a GitHub Release is
created. Re-running after a partial failure is safe: existing tags must resolve
to the same commit and an existing matching GitHub Release is returned unchanged.
`release-tags` owns immutable-tag verification and atomic creation; GOLFS only
selects the application and chart tag names and their release ordering.

Automated integration and provider-conformance suites are intentionally
deferred during the initial implementation phase. The protected Environment is
therefore the manual publication and deployment boundary; do not bypass it.
