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
- PUT actions bind expected size, OID-derived SHA-256 payload hash, and conditional creation into the signature.
- Hetzner rejects a PUT whose body does not match the signed `x-amz-content-sha256` value. Verify and download subsequently confirm existence and size without re-reading the body.
- The service never logs authorization headers, tokens, S3 credentials, signed URLs, or per-object OIDs.
- Metrics labels exclude repository, owner, OID, token, and URL values.
- The container runs as a non-root user with a read-only root filesystem and no Linux capabilities in the Helm defaults.

## Residual risks

- A stolen presigned URL and its required headers can be used until expiration. Keep the one-hour default or lower it.
- A compromised GOLFS process can use configured App and S3 credentials. Use workload isolation, Secret rotation, restricted S3 bucket permissions, and outbound network policy.
- A principal with direct S3 write credentials can bypass GOLFS's conditional upload path. Restrict those credentials to GOLFS and trusted operators, and independently validate any operator-restored object before making it visible.
- Per-process cache invalidation is bounded by 60 seconds. Permission revocation can remain effective in one replica until its entry expires.
- Objects are retained indefinitely. A malicious authorized uploader can consume storage within the configured object and request limits. Provider quotas and monitoring remain operator responsibilities.
- Provider claims of S3 compatibility are insufficient. Hetzner Object Storage is the currently validated provider; other providers require a new conformance decision before they are supported.
- Objects created by the former checksum-metadata upload contract are not trusted by this contract. Cut over from that version using an empty bucket or a separately validated migration.
