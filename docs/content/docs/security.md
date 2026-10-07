---
title: Security
description: Protected assets, trust boundaries, controls, and accepted security limitations.
---

GOLFS's security model describes the assets it protects, the trust boundaries it crosses, its controls, and its accepted limitations.

## Protected assets

- Git LFS object confidentiality and integrity
- GitHub user access tokens and App private key
- S3 credentials and presigned URLs
- Repository isolation by numeric GitHub ID
- Service availability and bounded resource use

## Trust boundaries

| Boundary                                           | Security requirement                                                                                                     |
| -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| Client and TLS ingress                             | Protect credentials and protocol requests over HTTPS.                                                                    |
| Ingress and GOLFS replicas                         | Keep the public request path separate from the internal operations listener.                                             |
| GOLFS replicas and GitHub API                      | Use HTTPS and authenticated GitHub responses to establish App installation, repository identity, and user permission.    |
| GOLFS replicas, clients, and S3 provider           | Use HTTPS for provider requests and direct object transfers; rely on the validated provider's signed-upload enforcement. |
| Internal operations listener and cluster workloads | Keep unauthenticated health, readiness, and metrics endpoints cluster-internal.                                          |

## Controls

### Credentials and authorization

- Every LFS operation requires Basic authentication. Public GitHub repository visibility never grants anonymous LFS access.
- Only `ghu_` GitHub App user access tokens are accepted. GOLFS accepts only GitHub-resolved installations in the operator allowlist, and installation matching binds the user token to that same repository installation.
- The service never logs authorization headers, tokens, S3 credentials, signed URLs, or per-object OIDs.

### Isolation and integrity

- Repository paths never form S3 keys. The canonical numeric repository ID resolved by GitHub is the namespace.
- OIDs are exactly 64 lowercase hexadecimal characters and cannot escape their prefix.
- PUT actions bind expected size, OID-derived SHA-256 payload hash, and conditional creation into the signature.
- Hetzner rejects a PUT whose body does not match the signed `x-amz-content-sha256` value. Verify and download subsequently confirm existence and size without re-reading the body.

### Workloads and networking

- Object size, request size, object count, HTTP headers, server timeouts, authorization cache size, and S3 concurrency are bounded.
- Metrics labels exclude repository, owner, OID, token, and URL values.
- The container runs as a non-root user with a read-only root filesystem and no Linux capabilities in the Helm defaults.
- TLS is required between clients and ingress and between GOLFS and external services. The operations listener is unauthenticated and must remain cluster-internal.

## Accepted limitations

- A stolen presigned URL and its required headers can be used until expiration. Keep the one-hour default or lower it.
- A compromised GOLFS process can use configured App and S3 credentials. Use workload isolation, Secret rotation, restricted S3 bucket permissions, and outbound network policy.
- A principal with direct S3 write credentials can bypass GOLFS's conditional upload path. Restrict those credentials to GOLFS and trusted operators, and independently validate any operator-restored object before making it visible.
- Per-process cache invalidation is bounded by 60 seconds. Previously granted access can remain usable in one replica after permission revocation until its entry expires.
- Objects are retained indefinitely. A malicious authorized uploader can consume storage within the configured object and request limits. Provider quotas and monitoring remain operator responsibilities.
- Provider claims of S3 compatibility are insufficient. Hetzner Object Storage is the currently validated provider; other providers require a new conformance decision before they are supported.
- Objects created by the former checksum-metadata upload contract are not trusted by this contract. Cut over from that version using an empty bucket or a separately validated migration.

## Further reading

The repository's [security policy](https://github.com/kode-blox/golfs/blob/main/SECURITY.md) is the canonical source for supported versions and private vulnerability reporting.

Use [Configuration](/configuration) to configure these boundaries, [Operations](/operations) for deployment and troubleshooting, and [Testing](/testing) for automated coverage and manual integration checks.
