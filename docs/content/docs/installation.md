---
title: Installation
description: Prepare storage, credentials, HTTPS routing, and a working GOLFS Helm deployment.
---

GOLFS runs as a stateless container with a private Hetzner Object Storage bucket. The included Helm chart deploys replicas, configuration, and separate public and operations Services. The commands below are operator steps to run against your chosen cluster from the repository root.

## Prepare the dependencies

1. Use a checkout of the release you intend to deploy, a Kubernetes cluster, Helm, and `kubectl`. The chart's default image is `ghcr.io/kode-blox/golfs`; an empty `image.tag` uses `charts/Chart.yaml.appVersion`. Chart and application versions are independent; see [Release process](/release-process) before overriding the image.
2. Create a private Hetzner bucket and credentials that permit bucket access checks, object metadata reads, uploads, and downloads. Record its HTTPS endpoint, signing region, and bucket name. Clients must also be able to reach the S3 endpoint because object bytes transfer directly there. Do not enable lifecycle deletion of LFS objects.
3. Follow the operator portion of [GitHub App and GCM setup](/github-app-and-gcm): enable Device Flow, keep expiring user tokens, install the App on selected repositories, and generate its RSA private key. Record the client ID and the installation IDs this deployment will admit.
4. Choose an HTTPS origin such as `https://lfs.example.com`, configure DNS, and prepare TLS termination. The public origin must have no path. Keep the operations Service inside the cluster.

For a bucket containing older GOLFS data, review [storage-layout cutover](/operations#storage-layout-cutover) before deployment. A new empty bucket avoids legacy-layout and integrity-contract migration requirements.

## Supply the runtime Secret

Create the `golfs` namespace and a Secret named `golfs` in that namespace through your platform's Secret-management process. The [chart README](https://github.com/kode-blox/golfs/blob/main/charts/README.md#required-secret) is the reference for required Secret keys, External Secrets configuration, and rotation behavior.

The App private key is always required as `GOLFS_GITHUB_APP_PRIVATE_KEY_PEM`. Static S3 credentials use `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`, with `AWS_SESSION_TOKEN` when applicable. An alternative AWS SDK credential provider can supply S3 credentials; it does not remove the App private-key requirement. Keep these values out of the chart values file and source control.

If using External Secrets instead, install its operator and CRDs first and configure the chart's Secret store and key mappings. The chart does not create external provider accounts or credentials.

## Configure and install the chart

Create a local `golfs-values.yaml`, replacing all example values with the prepared resources:

```yaml
environmentsVars:
  GOLFS_PUBLIC_URL: https://lfs.example.com
  GOLFS_GITHUB_APP_CLIENT_ID: Iv1_YOUR_CLIENT_ID
  GOLFS_GITHUB_ALLOWED_INSTALLATION_IDS: "12345678"
  GOLFS_S3_ENDPOINT: https://YOUR_HETZNER_S3_ENDPOINT
  GOLFS_S3_REGION: YOUR_SIGNING_REGION
  GOLFS_S3_BUCKET: YOUR_BUCKET
secret:
  existingSecret: golfs
```

[Configuration](/configuration) is the complete environment-variable reference. [Chart defaults](https://github.com/kode-blox/golfs/blob/main/charts/values.yaml) cover image selection, resources, probes, autoscaling, and optional resources.

The chart leaves Gateway resources disabled by default. Either route HTTPS traffic through your existing ingress to the public Service `golfs` on port `8080`, or add the following to the values file. The latter requires installed Gateway API CRDs, a controller supporting your chosen GatewayClass, and a TLS Secret in the `golfs` namespace:

```yaml
gateway:
  enabled: true
  className: YOUR_GATEWAY_CLASS
  annotations:
    cert-manager.io/cluster-issuer: null
  hosts:
    - host: lfs.example.com
      tlsSecretName: lfs-example-com-tls
```

The `null` value removes the chart's default cert-manager issuer annotation, leaving certificate provisioning to your existing process. If using cert-manager Gateway integration instead, configure that annotation with your actual ClusterIssuer. The generated HTTPRoute forwards `/github.com` paths to the public Service. It does not expose health, readiness, or metrics.

Install after the runtime Secret and routing prerequisites are ready:

```shell
helm upgrade --install golfs ./charts --namespace golfs --create-namespace --values golfs-values.yaml --wait
kubectl rollout status deployment/golfs --namespace golfs
```

For chart-managed namespaces and the alternative `--set` installation form, use the [chart installation reference](https://github.com/kode-blox/golfs/blob/main/charts/README.md#install).

## Confirm the deployment and connect a client

Startup validates the App, every allowed installation, and bucket access before readiness succeeds. If the rollout fails, inspect sanitized Deployment logs and the [Operations troubleshooting table](/operations#troubleshooting). A ready Pod proves startup validation, not a complete client transfer.

Configure GCM and the repository-specific LFS URL using [GitHub App and GCM setup](/github-app-and-gcm). Complete its device-flow gate and the [manual storage and transfer gates](/testing#manual-external-service-gates), including a real upload and a fresh-clone download. A `404` at the public origin's root is expected: GOLFS has no homepage.
