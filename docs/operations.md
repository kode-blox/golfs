# Operations

## Endpoints

`GET /healthz` reports only process liveness. `GET /readyz` becomes successful after startup validation and becomes unsuccessful during shutdown. `GET /metrics` exposes Prometheus metrics without authentication and must remain cluster-internal.

Metrics use low-cardinality labels for route, status, operation, and outcome. They never identify an owner, repository, object, token, or signed URL.

## Retention and backup

Objects are immutable through GOLFS and retained indefinitely. Do not configure bucket lifecycle deletion. Back up the entire bucket using provider replication appropriate to your recovery objectives. Before restoring an object at its exact repository-ID and shard key, independently confirm that its bytes hash to its full OID; GOLFS subsequently checks existence and size without re-reading the body.

The GitHub App private key and S3 credentials must be backed up through the platform's Secret-management process, not alongside object data.

## Troubleshooting

| Symptom | Meaning |
|---|---|
| `401` with `LFS-Authenticate` | Missing, malformed, expired, non-`ghu_`, or wrong-App credential |
| `403` on upload or Verify | User has pull but not push permission |
| `404` | Repository is inaccessible, App is not installed, or requested object is absent |
| `422` | Request validation failed or existing object metadata conflicts |
| `429` | GitHub rate limit, retry after the response delay |
| `503` | GitHub or S3 is temporarily unavailable |

Top-level errors include `X-Request-ID` and a matching JSON `request_id`. Use this ID to correlate sanitized application logs. Per-object Batch errors deliberately do not fail successful objects in the same Batch.

An existing key with a different size is an integrity incident. GOLFS will not overwrite it. Inspect and repair or remove the exact key directly in S3 under an approved operator procedure, then retry the Batch. GOLFS has no deletion API or administrative command.

## Storage-layout cutover

Object keys use `github/{repository-id}/objects/{first-two}/{next-two}/{full-oid}`. Flat keys created by the earlier implementation are not read. Before deploying this layout against an existing bucket, stop uploads and either empty the bucket or perform a separately validated out-of-band migration. Do not mix legacy flat objects with objects accepted through the signed-payload contract.

## Graceful shutdown

On SIGINT or SIGTERM, GOLFS first makes readiness false, then gives both listeners up to 30 seconds to drain. Kubernetes should keep the default termination grace period at or above 30 seconds.
