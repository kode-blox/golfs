# Release process

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
- `validate-container.yaml` performs the pull-request and manually dispatched
  disposable container build with publishing disabled.
- `deliver-development.yaml` prepares, deploys, and verifies one development
  delivery, then returns the verified image digest required by production.
- `prepare-development.yaml` builds the main commit once, publishes
  `build-<full SHA>`, and returns its image reference, tag, and digest. In
  parallel, it reads the development GitOps wrapper and compares `charts/**`
  from the deployed chart's source ref to the current commit. It publishes a
  `0.0.0-build-<full lowercase SHA>` development chart with
  `appVersion: build-<full SHA>` only when that comparison finds chart changes;
  otherwise its chart-version output is empty.
- `release-eligibility.yaml` validates the independent version authorities and
  checks the `v<application version>` and `chart-v<chart version>` release tags
  independently after development verification.
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

## Pipeline graph

A pull request runs only:

```text
CI -> disposable container validation (push=false)
```

A manual dispatch runs only the same validation path for the selected ref:

```text
ci -> validate-container (push=false)
```

The top-level pipeline contains exactly these six business stages:

```text
ci
validate-container
deliver-development
release-eligibility
deliver-production
release-records
```

Every applicable push to `main` runs:

```text
CI
  -> deliver development
     (prepare, deploy, and verify;
      preparation builds the container and reads development state in parallel)
  -> determine release eligibility
```

An image-only development delivery deliberately skips development chart
publication. The development deployment still runs with the new image tag and
an empty chart-version input. A chart-changing delivery passes both the image
tag and published development chart version to the same deployment call.

If both immutable release tags already exist, the run is an ordinary main
delivery and ends successfully after `release-eligibility`. Otherwise, only the
required stable outputs are prepared:

```text
application pending: promote the verified digest to <application version>
chart pending:       publish <chart version> with appVersion=<application version>
                     -> deploy production once
                     -> verify production
                     -> create release records
```

The application and chart decisions are independent:

| Delivery | Stable container promotion | Stable chart publication | Production update | Release records |
| --- | --- | --- | --- | --- |
| Ordinary main | Skipped | Skipped | Skipped | None |
| Application only | Application version | Skipped | Image tag only | Application only |
| Chart only | Skipped | Chart version | Chart version only | Chart only |
| Both | Application version | Chart version | Image and chart together | Application and chart |

`release-eligibility` is the read-only production gate. A successful result with
`release-needed` false skips production delivery and release records;
`release-needed` true permits `deliver-production` to prepare, deploy, and verify
the selected releases. An eligibility or delivery failure blocks release-record
creation.

## Container, chart, tag, and release conventions

Container tags never contain a `v` prefix:

- Development: `build-<full SHA>`
- Stable application: `<application version>`
- Mutable `latest`: never created

The image digest is an internal handoff from the one build to stable promotion.
GitOps receives a tag, not the digest. The workflows do not perform client-side
tag-immutability checks; any registry-side controls are separate and depend on
the selected registry's supported, configured features.

Development charts use `0.0.0-build-<full lowercase SHA>`. Stable charts use the
plain `charts/VERSION` value. Chart-only releases preserve the production image
tag because the deployment receives an empty image-tag input.

Git tags and GitHub Releases use these separate families:

| Family | Git tag | GitHub Release name | Release-note pathspecs |
| --- | --- | --- | --- |
| Application | `v<application version>` | `GOLFS v<application version>` | Top-level repository content excluding `charts/**` |
| Chart | `chart-v<chart version>` | `GOLFS chart v<chart version>` | `charts/**` only |

`release-tags@v1` ensures the selected tags in one non-force operation.
`create-release@v1` runs only after that operation succeeds. Re-running after a
partial failure is safe: existing immutable tags must resolve to the same
commit, and an existing matching GitHub Release is returned unchanged.

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
updated by `chart-update-deploy@v1`. The development chart also records the
source ref consumed by `prepare-development.yaml`.

The deployment workflow changes only the selected wrapper. It does not modify
ApplicationSets, bootstrap manifests, repository settings, or the live cluster.
Argo CD observes the GitOps commit and reconciles the selected Environment.
