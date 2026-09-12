---
title: Release process
description: Application and chart version authorities, staged delivery, manual delivery, and release records.
---

The root `VERSION` file is the current application release authority.
`charts/VERSION` is the independent chart version authority.
`charts/Chart.yaml.appVersion` records the application version associated with
the authored chart and may differ from the current root `VERSION`.
`charts/Chart.yaml.version` must match the chart version.

Use the manually dispatched **Bump Version** workflow for routine releases. The
`application` target bumps only the application authority. The `chart` target
bumps only the chart authority and synchronizes `Chart.yaml.appVersion` to the
current application version. The `both` target bumps both authorities and
synchronizes `appVersion` to the new application version. The workflow pushes
with a GitHub App installation token so the resulting `main` push starts the
delivery pipeline.

## Workflow ownership

`.github/workflows/pipeline.yaml` is the thin orchestrator. It owns triggers,
top-level phase ordering and permissions, concurrency, release policy, and data
passed between delivery phases. The delivery workflows own the job ordering,
permissions, and Environment selection within each phase. All same-repository
calls use the `$/` self-repository form so the called workflow comes from the
exact commit running the pipeline.

The implementation remains split by responsibility:

- `ci.yaml` validates the Go source, Helm chart, vulnerabilities, formatting,
  lint, and workflow syntax.
- `detect-delivery-changes.yaml` resolves the native event range through
  `Resolve-DeliveryEventRange.ps1`, then classifies application, chart,
  documentation, and release-authority changes.
- `validate-container.yaml` performs the pull-request and manually dispatched
  disposable container build with publishing disabled.
- `deliver-documentation.yaml` builds and validates the documentation site,
  then deploys it for pushes and manual runs selected from `main`.
- `deliver-development.yaml` prepares the selected development artifacts and,
  when requested, deploys and verifies them. It returns any prepared image
  digest required by production.
- `prepare-development.yaml` publishes only the selected application and chart
  artifacts from the current `github.sha`, returning any published artifact
  outputs.
- `release-eligibility.yaml` validates the independent version authorities and
  guards requested stable releases with independent
  `v<application version>` and `chart-v<chart version>` tag probes after
  development verification.
- `deliver-production.yaml` prepares, deploys, and verifies one production
  delivery for the releases identified as pending.
- `prepare-production.yaml` promotes the verified digest and publishes the stable
  chart in parallel for the releases identified as pending.
- `deploy.yaml` updates one GitOps wrapper with an optional chart version, an
  optional image tag, or both in one commit, then returns that commit SHA.
- `verify-deployment.yaml` verifies that the exact GitOps commit was synchronized
  by the selected Argo CD Application.
- `create-release-records.yaml` creates only the pending immutable Git tags, then
  creates the matching GitHub Releases after tag creation succeeds.
- `manual-deliver-development.yaml` runs CI for the selected branch or Git tag,
  then explicitly publishes or reuses the selected development artifacts and
  optionally deploys them.
- `manual-deliver-production.yaml` validates release metadata and stable-tag
  conflicts for the selected branch or Git tag, resolves the selected
  development image when required, then runs production delivery and
  release-record creation without re-running CI, development delivery, or
  release eligibility.

## Pipeline graph

A pull request runs:

```text
detect delivery changes -> build and validate documentation when selected
CI -> disposable container validation (push=false)
```

A manual dispatch of `pipeline.yaml` runs the same validation path for the
selected ref. When documentation changes are selected, it also builds and
validates the site; a dispatch from `main` deploys that site:

```text
detect delivery changes -> build documentation when selected
ci -> validate-container (push=false)
```

The top-level pipeline contains these eight business stages:

```text
ci
detect-delivery-changes
deliver-documentation
validate-container
deliver-development
release-eligibility
deliver-production
release-records
```

Every applicable push to `main` runs:

```text
select application, chart, documentation, and release-intent changes from
github.event.before..github.event.after
  -> build and deploy documentation when selected
CI
  -> prepare, deploy, and verify the selected development components
  -> guard any stable release requested in that same pushed range
```

The push range is always the full `github.event.before..github.event.after`
range, so a multi-commit push is evaluated as one event. Selection determines
which components to deliver; it does not select an older source revision. Every
selected component is built or packaged from the final current `github.sha` and
uses that full SHA in its development identity.

The normal development cases are:

| Current push range | Development preparation | GitOps update |
| --- | --- | --- |
| Application only | Build or reuse `build-<full github.sha>` | Image tag only |
| Chart only | Package or reuse `0.0.0-build-<full github.sha>` while preserving the authored `charts/Chart.yaml.appVersion` | Chart version only |
| Application and chart | Use `build-<full github.sha>` and `0.0.0-build-<full github.sha>`, overriding the development chart `appVersion` with the same image tag | Image and chart together in one atomic commit |
| Neither | Stop after CI | None |

`chart-update-deploy@v1` receives only the selected outputs. When both
components are selected it applies both values in one GitOps commit, preserving
the atomic application-and-chart deployment boundary.

## Exact-reference idempotency and manual delivery

A rerun of the same push at the same `github.sha` derives the same exact
development references. If a selected current reference already exists, the
publication action reuses it or skips republishing it and continues with the
same identity. This makes an exact-reference same-HEAD rerun idempotent.

Normal pushes do not use GitOps deployment state as a publication baseline, do
not resolve source history, and do not select or automatically publish missed
artifacts from older revisions. Exact-reference reuse is therefore not
historical auto-healing.

Manual delivery is explicit and remains selected-commit scoped:

- `manual-deliver-development.yaml` accepts `application`, `chart`, or `both`.
  It runs CI for the branch or Git tag selected at dispatch, forces publication
  or exact-reference reuse of the selected development artifacts, and deploys
  and verifies them through the `development` Environment by default. Setting
  `deploy` to false stops after preparation and publication.
- `manual-deliver-production.yaml` accepts the same component and ref choices.
  Application delivery requires the selected `build-<github.sha>` container;
  chart delivery packages the stable chart directly from the selected ref and
  does not require a development chart. The workflow validates the independent
  version authorities and stable Git tag conflicts, then sends the selected
  components through the `production` Environment by default before creating
  release records. Setting `deploy` to false skips GitOps deployment and
  verification but still prepares the stable artifacts and creates their
  release records. The workflow does not call CI, development delivery, or
  `release-eligibility.yaml`.

Neither manual delivery workflow infers a different source revision or scans
for missed historical work. Each operates on the exact `github.sha` resolved
from the branch or Git tag selected at dispatch.

## Stable release intent and delivery

Stable release intent comes only from authority-file changes in the current
`github.event.before..github.event.after` push range:

- A root `VERSION` change requests an application release.
- A `charts/VERSION` change requests a chart release.
- Both changes request both releases.

Other application or chart changes never imply a stable release. For each
requested family, the immutable tag probe is only a guard:

- A missing tag makes the current release eligible.
- A tag already at the current `github.sha` means that release is complete and
  is skipped.
- A tag at another commit is a version conflict and fails the run.

Only eligible stable outputs are prepared:

```text
application eligible: promote the resolved development digest to <application version>
                      without rebuilding
chart eligible:       publish <chart version> while preserving the authored
                      charts/Chart.yaml.appVersion
                     -> deploy production once
                     -> verify production
                     -> create tags and releases at the current github.sha
```

The application and chart decisions are independent:

| Delivery | Stable container promotion | Stable chart publication | Production update | Release records |
| --- | --- | --- | --- | --- |
| Ordinary main | Skipped | Skipped | Skipped | None |
| Application only | Application version | Skipped | Image tag only | Application only |
| Chart only | Skipped | Chart version | Chart version only | Chart only |
| Both | Application version | Chart version | Image and chart together | Application and chart |

`release-eligibility.yaml` is the read-only production gate. A successful result
with `release-needed` false skips production delivery and release records;
`release-needed` true permits `deliver-production.yaml` to prepare, deploy, and
verify the selected releases. An eligibility or delivery failure blocks
release-record creation.

## Container, chart, tag, and release conventions

Container tags never contain a `v` prefix:

- Development: `build-<full SHA>`
- Stable application: `<application version>`
- Mutable `latest`: never created

The image digest is an internal handoff from development publication or exact
reference reuse to stable promotion. Application production adds the plain
`<application version>` tag to that resolved digest without rebuilding. GitOps
receives a tag, not the digest. The workflows do not perform client-side
tag-immutability checks; any registry-side controls are separate and depend on
the selected registry's supported, configured features.

Development charts use `0.0.0-build-<full lowercase SHA>`. Stable charts use the
plain `charts/VERSION` value and preserve the authored
`charts/Chart.yaml.appVersion`. Chart-only releases preserve the production
image tag because the deployment receives an empty image-tag input.

Git tags and GitHub Releases use these separate families:

| Family | Git tag | GitHub Release name | Release-note pathspecs |
| --- | --- | --- | --- |
| Application | `v<application version>` | `GOLFS v<application version>` | Top-level repository content excluding `charts/**` |
| Chart | `chart-v<chart version>` | `GOLFS chart v<chart version>` | `charts/**` only |

`release-tags@v1` ensures the selected tags in one non-force operation.
`create-release@v1` runs only after that operation succeeds. Re-running after a
partial failure is safe: existing immutable tags must resolve to the same
current `github.sha`, and an existing matching GitHub Release is returned
unchanged. Both Git tags and their GitHub Releases therefore target the current
pipeline commit.

## Deferred supply-chain work

Artifact provenance and signed attestations remain deferred. The current
delivery graph does not claim either capability.

## Concurrency

The pipeline concurrency group separates push, pull-request, and manual runs.
Pull-request groups are pull-request-specific; push and manual groups are
ref-specific within their event type. A newer pull-request or manual run may
cancel an older matching run. Push runs never cancel an in-progress delivery,
keeping publication, GitOps updates, verification, and release records
serialized.

## Required repository and Environment configuration

The workflows reference these settings; they do not create or change them.

Repository configuration:

- Variable `GITOPS_APP_ID` with the GitHub App ID.
- Secret `GITOPS_APP_PRIVATE_KEY` with the App private key.
- The GitHub App has repository `contents: write` permission and is installed
  for both `kode-blox/golfs` and
  `SayakMukhopadhyay/k8s-landscape-charts`.
- Repository and organization Actions policies allow the pinned third-party
  actions, `SayakMukhopadhyay/github-actions@v1`, self-repository reusable
  workflows, and the explicitly requested `GITHUB_TOKEN` permissions.
- These workflows intentionally do not implement client-side tag immutability
  checks. Registry-side controls remain separate when the selected registry
  supports and is configured for them.

The `development` Environment provides:

- Variable `GITOPS_ENVIRONMENT` set to `hetzner-fsn1-dc4-prod/dev`.
- Variable `ARGOCD_SERVER` with the Argo CD server hostname.
- Variable `ARGOCD_APPLICATION` set to
  `golfs-dev-hetzner-fsn1-dc4-prod`.
- Variable `URL` with the development deployment URL.
- Secret `ARGOCD_AUTH_TOKEN` with the read-only Argo CD token.
- Secrets `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET` with the
  verifier's Cloudflare Access service-token credentials.

The `production` Environment provides:

- Variable `GITOPS_ENVIRONMENT` set to `hetzner-fsn1-dc4-prod/prod`.
- Variable `ARGOCD_SERVER` with the Argo CD server hostname.
- Variable `ARGOCD_APPLICATION` set to
  `golfs-prod-hetzner-fsn1-dc4-prod`.
- Variable `URL` with the production deployment URL and release-record URL.
- Secret `ARGOCD_AUTH_TOKEN` with the read-only Argo CD token.
- Secrets `CF_ACCESS_CLIENT_ID` and `CF_ACCESS_CLIENT_SECRET` with the
  verifier's Cloudflare Access service-token credentials.
- Secret `OPENAI_API_KEY` for GitHub Release descriptions.
- Required reviewers and any other deployment protection rules for the
  production approval boundary.

The GitOps repository contains one wrapper per Environment:

- `golfs/envs/hetzner-fsn1-dc4-prod/dev`
- `golfs/envs/hetzner-fsn1-dc4-prod/prod`

Each wrapper contains exactly one dependency named `golfs`, commits its
`Chart.lock` and vendored dependency archive, and supports the image-tag value
updated by `chart-update-deploy@v1`.

The deployment workflow changes only the selected wrapper. It does not modify
ApplicationSets, bootstrap manifests, repository settings, or the live cluster.
Argo CD observes the GitOps commit and reconciles the selected Environment.
