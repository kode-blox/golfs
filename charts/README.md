# GOLFS Helm chart

This chart deploys the stateless server, separate application and metrics Services, an
optional Gateway API route, and optional External Secrets and ServiceMonitor
resources.

## Required secret

By default, `secret.existingSecret` must name a Secret containing the GitHub App
private key and, when the AWS SDK default credential chain is not being used,
the S3 credentials:

```text
GOLFS_GITHUB_APP_PRIVATE_KEY_PEM (always required)
AWS_ACCESS_KEY_ID (when using static credentials)
AWS_SECRET_ACCESS_KEY (when using static credentials)
AWS_SESSION_TOKEN (when required)
```

Set `externalSecrets.enabled=true` to let the chart create an ExternalSecret
instead. The External Secrets Operator CRDs must already be installed. The
mapping list must contain `GOLFS_GITHUB_APP_PRIVATE_KEY_PEM`; AWS credentials
may instead come from the AWS SDK default credential chain.

Secret values injected through `envFrom` are read only when a container starts.
After rotating either an existing Secret or an ExternalSecret-managed Secret,
restart the Deployment. A Secret-reloader controller may automate that restart;
configure it through `podAnnotations` when one is installed in the cluster.

The default GitHub App client ID is a lintable placeholder. Every real install
must override `environmentsVars.GOLFS_GITHUB_APP_CLIENT_ID` and
`environmentsVars.GOLFS_GITHUB_ALLOWED_INSTALLATION_IDS`. The latter is a
comma-separated allowlist of GitHub-issued installation IDs and is not secret.

## Install

```shell
helm upgrade --install golfs ./charts \
  --namespace golfs \
  --create-namespace \
  --set environmentsVars.GOLFS_PUBLIC_URL=https://lfs.example.com \
  --set environmentsVars.GOLFS_GITHUB_APP_CLIENT_ID=Iv1_example \
  --set-string environmentsVars.GOLFS_GITHUB_ALLOWED_INSTALLATION_IDS=12345678 \
  --set environmentsVars.GOLFS_S3_ENDPOINT=https://s3.example.com \
  --set environmentsVars.GOLFS_S3_REGION=us-east-1 \
  --set environmentsVars.GOLFS_S3_BUCKET=golfs \
  --set secret.existingSecret=golfs
```

Set `namespace.create=true` when a GitOps or rendered-manifest workflow should
create `.Release.Namespace` from the chart. Optional `namespace.labels` and
`namespace.annotations` are applied to it. The chart does not retain the
Namespace by default. Supply the retention annotation explicitly when that
lifecycle policy is desired:

```yaml
namespace:
  annotations:
    helm.sh/resource-policy: keep
```

For a direct Helm install into a namespace that does not exist yet, continue to
pass `--create-namespace`. Helm must create its release namespace before it can
apply chart templates, so a chart-managed Namespace cannot bootstrap that Helm
operation by itself.

The Gateway and HTTPRoute target only the application Service. Keep the metrics
Service cluster-internal because it exposes unauthenticated health, readiness,
and metrics endpoints. The chart therefore accepts only `ClusterIP` for
`metricsService.type`.
