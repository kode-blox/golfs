# Threat model

## Protected assets

- Git LFS object confidentiality and integrity
- GitHub user access tokens and App private key
- S3 credentials and presigned URLs
- Repository isolation by numeric GitHub ID
- Service availability and bounded resource use

## Trust boundaries

The client, ingress, GOLFS replicas, GitHub API, and S3 provider are separate trust domains. TLS is required between clients and ingress and between GOLFS and external services. The operations listener is unauthenticated and must remain cluster-internal.

## Controls

- Every LFS operation requires Basic authentication. Public GitHub repository visibility never grants anonymous LFS access.
- Only `ghu_` GitHub App user access tokens are accepted. Installation matching binds the user token to the configured App's repository installation.
- Repository paths never form S3 keys. The canonical numeric repository ID resolved by GitHub is the namespace.
- OIDs are exactly 64 lowercase hexadecimal characters and cannot escape their prefix.
- Object size, request size, object count, HTTP headers, server timeouts, authorization cache size, and S3 concurrency are bounded.
- PUT actions bind expected size, SHA-256 checksum, and conditional creation into the signature.
- Verify and download independently compare S3 checksum metadata and size.
- The service never logs authorization headers, tokens, S3 credentials, signed URLs, or per-object OIDs.
- Metrics labels exclude repository, owner, OID, token, and URL values.
- The container runs as a non-root user with a read-only root filesystem and no Linux capabilities in the Helm defaults.

## Residual risks

- A stolen presigned URL and its required headers can be used until expiration. Keep the one-hour default or lower it.
- A compromised GOLFS process can use configured App and S3 credentials. Use workload isolation, Secret rotation, restricted S3 bucket permissions, and outbound network policy.
- Per-process cache invalidation is bounded by 60 seconds. Permission revocation can remain effective in one replica until its entry expires.
- Objects are retained indefinitely. A malicious authorized uploader can consume storage within the configured object and request limits. Provider quotas and monitoring remain operator responsibilities.
- Provider claims of S3 compatibility are insufficient. During the initial implementation phase, operators must manually validate the required integrity and atomicity behavior before treating a provider as supported.
