# Architecture

## Boundaries

The protocol handler depends only on `forge.Authorizer` and `storage.ObjectStore`. GitHub REST types remain inside the GitHub adapter, and AWS SDK types remain inside the S3 adapter. Future forge or storage adapters can be added without changing Git LFS wire shapes.

GOLFS stores no durable state. Each replica has only a bounded, positive authorization cache with a 60-second lifetime. Cache keys are HMACs of the user token and normalized repository path using a random per-process key; raw tokens are never cached.

## Repository identity

GitHub's numeric repository ID is the storage namespace. The path supplied by the client is used for authorization and can change after a rename without moving objects. The object key is:

```text
github/{repository-id}/objects/{oid[0:2]}/{oid[2:4]}/{oid}
```

An App installation is the repository-admission policy. Public visibility does not bypass authentication or App installation checks.

## Upload

GOLFS performs `HeadObject` before creating an upload action. An existing object with the requested size is complete and receives no actions. An absent object receives a one-hour presigned `PutObject` with signed `Content-Length`, `x-amz-content-sha256`, and `If-None-Match: *`, plus an authenticated Verify action. The content hash is the validated 64-character Git LFS OID in hexadecimal form and is included in both the SigV4 canonical request and the upload action's required headers.

The client sends bytes directly to Hetzner Object Storage. Hetzner rejects a body that does not match the signed payload hash. After an accepted PUT, Verify performs another `HeadObject` and requires the object to exist with the requested size. A key with a different size is never overwritten automatically; an S3 operator must repair it out of band.

## Download

GOLFS requires pull permission, checks the stored size, and returns a presigned `GetObject`. The Batch object is marked authenticated so Git LFS does not send GOLFS credentials to the S3 host.

## Storage trust

GOLFS trusts objects because Hetzner enforces the OID-derived payload hash when they are created and GOLFS-issued uploads use conditional creation. Direct S3 credentials are therefore restricted to trusted operators. GOLFS does not proxy or re-download object bodies to hash them, and it does not use a persistent marker.

Existing flat-layout objects are not discovered through the sharded key mapping. A deployment changing from the flat layout must begin with an empty bucket or complete an independently validated out-of-band migration. GOLFS has no dual-read fallback or automatic migration.

## Failure and retry model

GitHub rate limits become `429` with `Retry-After`. GitHub and S3 availability failures become retryable `503` errors. S3 failures that affect only one object in an otherwise valid Batch are returned as per-object errors so successful objects are preserved.

Direct actions are retry-safe:

- A client that loses a successful PUT response can Verify or re-Batch; an object with the expected size converges to complete.
- Concurrent uploads use conditional creation. At most one PUT can create the key, and later Batches recognize the winning object by its size.
- Expired signed URLs require a new Batch and do not change object identity.
- A temporary Verify failure can be retried without uploading bytes again.

## Retention and forks

GOLFS never deletes objects. Force pushes, rebases, and deleted Git refs do not reclaim storage. S3 lifecycle rules that delete LFS objects are unsupported.

Fork provisioning is outside v0.1. A future operator workflow may copy the complete source prefix to the fork repository's numeric-ID prefix while preserving every OID shard path, full OID, and object body.
