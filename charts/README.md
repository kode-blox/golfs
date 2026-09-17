# GOLFS Helm chart

This chart deploys the stateless server, separate application and metrics Services, an
optional Gateway API route, and optional External Secrets and ServiceMonitor
resources.

## Required secret

By default, `secret.existingSecret` must name a Secret containing:

```text
GOLFS_GITHUB_APP_PRIVATE_KEY_PEM
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
AWS_SESSION_TOKEN (when required)
```

Set `externalSecrets.enabled=true` to let the chart create an ExternalSecret
instead. The External Secrets Operator CRDs must already be installed.

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

The Gateway and HTTPRoute target only the application Service. Keep the metrics
Service cluster-internal because it exposes health, readiness, and metrics.
