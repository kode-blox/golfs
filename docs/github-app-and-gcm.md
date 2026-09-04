# GitHub App and GCM setup

## Create the GitHub App

Create one GitHub App owned by the organization operating GOLFS:

- Enable Device Flow.
- Keep expiring user access tokens enabled.
- Grant repository Metadata permission as read-only. GitHub includes Metadata for every App.
- Do not subscribe to webhooks.
- Install the App only on repositories admitted to this GOLFS deployment.
- Generate an RSA private key and store it only in the GOLFS runtime Secret.

Use the App client ID for both `GOLFS_GITHUB_APP_CLIENT_ID` and client-side GCM configuration. Do not use the numeric App ID and do not distribute a client secret.

## Configure GCM 2.9.0 generic OAuth

Replace `lfs.example.com` and `Iv1_YOUR_CLIENT_ID`. Configure the origin that hosts GOLFS, not `github.com`:

```shell
git config --global credential.https://lfs.example.com.provider generic
git config --global credential.https://lfs.example.com.oauthClientId Iv1_YOUR_CLIENT_ID
git config --global credential.https://lfs.example.com.oauthAuthorizeEndpoint https://github.com/login/oauth/authorize
git config --global credential.https://lfs.example.com.oauthTokenEndpoint https://github.com/login/oauth/access_token
git config --global credential.https://lfs.example.com.oauthDeviceEndpoint https://github.com/login/device/code
git config --global credential.https://lfs.example.com.oauthAuthModes devicecode
git config --global credential.https://lfs.example.com.oauthUseClientAuthHeader false
git config --global credential.https://lfs.example.com.oauthDefaultUserName GOLFS
```

`oauthAuthModes=devicecode` prevents GCM from offering the browser authorization-code flow and makes this contract specifically exercise device flow. Do not configure `oauthClientSecret`. GitHub's device flow and refresh flow do not require a secret for tokens originally issued through device flow. GitHub App user access tokens start with `ghu_`, expire after eight hours, and are accompanied by a rotating refresh token when expiration is enabled. GCM stores and refreshes this credential.

Configure each repository's LFS endpoint:

```shell
git config --local lfs.url "https://lfs.example.com/github.com/OWNER/REPOSITORY/info/lfs"
```

The next LFS operation should display GitHub's device authorization prompt. Complete it as the intended user. The username GCM supplies is ignored by GOLFS; only the password token is authoritative.

## Required live gate

Before the first release, and after material changes to GCM, GitHub device flow, or token refresh behavior:

1. Remove any stored GOLFS credential from the test machine.
2. Run a real LFS fetch or push with GCM 2.9.0.
3. Complete device authorization without a client secret.
4. Confirm the server receives a `ghu_` token and accepts it only for the configured App installation.
5. Confirm GCM refreshes an expired access token without another login.
6. Confirm PAT, another App's `ghu_` token, and a user outside the installation all fail closed.

If this exact secretless flow fails, stop release work. Do not add a token issuer or PAT fallback without a new architecture decision.

References: [GCM generic OAuth](https://github.com/git-ecosystem/git-credential-manager/blob/main/docs/generic-oauth.md), [GitHub App device flow](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app), and [refreshing user access tokens](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/refreshing-user-access-tokens).
