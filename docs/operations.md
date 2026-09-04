# Operations

## Endpoints

`GET /healthz` reports only process liveness. `GET /readyz` becomes successful after startup validation and becomes unsuccessful during shutdown. `GET /metrics` exposes Prometheus metrics without authentication and must remain cluster-internal.

Metrics use low-cardinality labels for route, status, operation, and outcome. They never identify an owner, repository, object, token, or signed URL.

## Retention and backup

Objects are immutable and retained indefinitely. Do not configure bucket lifecycle deletion. Back up the entire bucket with versioning or provider replication appropriate to your recovery objectives. Restoring an object at the same repository-ID key restores GOLFS visibility after metadata validation.

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

An existing key with mismatched size or checksum is an integrity incident. GOLFS will not overwrite it. Inspect and repair or remove the exact key directly in S3 under an approved operator procedure, then retry the Batch. GOLFS has no deletion API or administrative command.

## Graceful shutdown

On SIGINT or SIGTERM, GOLFS first makes readiness false, then gives both listeners up to 30 seconds to drain. Kubernetes should keep the default termination grace period at or above 30 seconds.
