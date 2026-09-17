---
title: Configuration
description: Environment variables, startup validation, listeners, limits, and runtime configuration.
---

GOLFS reads configuration from environment variables and fails startup before readiness if required values are missing or unsafe.

## Required

| Variable                                | Meaning                                                  |
| --------------------------------------- | -------------------------------------------------------- |
| `GOLFS_PUBLIC_URL`                      | HTTPS origin clients use to reach GOLFS, without a path  |
| `GOLFS_GITHUB_APP_CLIENT_ID`            | GitHub App client ID, not numeric App ID                 |
| `GOLFS_GITHUB_ALLOWED_INSTALLATION_IDS` | Comma-separated allowlist of GitHub App installation IDs |
| `GOLFS_GITHUB_APP_PRIVATE_KEY_PEM`      | RSA private key PEM for App JWTs                         |
| `GOLFS_S3_ENDPOINT`                     | Absolute Hetzner Object Storage S3 API endpoint          |
| `GOLFS_S3_REGION`                       | Signing region                                           |
| `GOLFS_S3_BUCKET`                       | Existing private bucket                                  |
| `AWS_ACCESS_KEY_ID`                     | S3 access key                                            |
| `AWS_SECRET_ACCESS_KEY`                 | S3 secret key                                            |

`AWS_SESSION_TOKEN` is supported when credentials are temporary. The AWS SDK default credential chain remains available for workload identity and other standard providers.

## Optional

| Variable                  |      Default | Constraint                                                     |
| ------------------------- | -----------: | -------------------------------------------------------------- |
| `GOLFS_HTTP_ADDR`         |      `:8080` | Must differ from operations address                            |
| `GOLFS_ADMIN_ADDR`        |      `:9090` | Must differ from public address                                |
| `GOLFS_S3_USE_PATH_STYLE` |      `false` | Set only when the Hetzner endpoint requires it                 |
| `GOLFS_PRESIGN_TTL`       |         `1h` | Greater than zero, at most 168 hours                           |
| `GOLFS_MAX_OBJECT_SIZE`   | `5000000000` | Positive and configurable only downward                        |
| `GOLFS_LOG_LEVEL`         |       `info` | `debug`, `info`, `warn`, or `error`                            |
| `GOLFS_DEV_ALLOW_HTTP`    |      `false` | Allows HTTP public and S3 endpoints for local development only |

The public URL cannot contain credentials, a query, fragment, or non-root path. Production public and S3 endpoints must use HTTPS.

## Startup and readiness

Startup parses and validates the App private key, calls GitHub `GET /app` using a short-lived RS256 JWT, verifies that every allowed installation ID belongs to the configured App, and checks S3 bucket access. Readiness becomes true only after these checks succeed. Installation IDs are identifiers rather than secrets, but GOLFS accepts them only from its runtime configuration and GitHub's authenticated API responses.

After startup, readiness is local. A shared GitHub or S3 outage does not mark every replica unready; affected LFS requests return retryable errors. Readiness becomes false immediately when graceful shutdown begins.
