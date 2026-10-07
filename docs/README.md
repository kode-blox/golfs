# GOLFS documentation site

The documentation website uses the `fumadocs-mdx` Macro API, Fumadocs Core and Base UI (`fumadocs-ui` aliases `@fumadocs/base-ui`), and a statically exported Next.js application. Authored documentation lives in `content/docs`.

Collections are defined in `lib/source.ts` with lazy MDX bodies and processed Markdown. The Next.js integration compiles the macros without a collection-codegen or postinstall step. Shared URL helpers retain the root-hosted Markdown and Open Graph routes.

The landing page and documentation pages use Fumadocs layouts, documentation pages expose copy and source-view controls, and the static export includes search, Open Graph images, and machine-readable Markdown routes.

## Reader flow

The landing page starts at Installation. `content/docs/meta.json` defines the shared sidebar order:

- **Get started:** [Installation](content/docs/installation.md), [Configuration](content/docs/configuration.md), [GitHub App and GCM setup](content/docs/github-app-and-gcm.md).
- **Understand the system:** [Architecture](content/docs/architecture.md), [Security](content/docs/security.md), [Language and foundations](content/docs/language-and-foundations.md).
- **Operate and develop:** [Operations](content/docs/operations.md), [Testing](content/docs/testing.md), [Release process](content/docs/release-process.md).

Canonical filenames follow page titles: `security.md` serves Security at `/security`, and `release-process.md` serves Release process at `/release-process`. Root [SECURITY.md](../SECURITY.md) owns private reporting and supported-version policy; the website owns the technical security model. Keep deployment examples consistent with the [chart README](../charts/README.md), and verify behavioral claims against source and workflows when editing pages.

The shared Security outline uses these exact H2 headings, in order: **Protected assets**, **Trust boundaries**, **Controls**, **Accepted limitations**, **Further reading**. Protected assets is a bullet list. Trust boundaries is a table with **Boundary** and **Security requirement** columns. Controls uses these exact H3 headings, in order: **Credentials and authorization**, **Isolation and integrity**, **Workloads and networking**. Retain GOLFS-specific facts under this outline; link to the root policy and Testing rather than reproducing policy or coverage prose.

## Author and validate

Use the Node version in `.nvmrc`. From the repository root:

```shell
npm ci
npm run dev --workspace @golfs/docs
```

The local site is available at `http://localhost:3000`. Validate the production export with:

```shell
npm run lint --workspace @golfs/docs
npm run types:check --workspace @golfs/docs
npm run build --workspace @golfs/docs
```

The GitHub Pages site is served from the root of `https://golfs.kodeblox.com/`. Configure that custom domain in the repository's Pages settings; the Actions-based deployment does not require a `CNAME` file.

The production export includes these machine-readable entry points:

- `llms.txt` for the documentation index.
- `llms-full.txt` for the complete documentation corpus.
- `llms.mdx/docs/<page>/content.md` for an individual page.

After navigation or content changes, check internal links in `docs/out`, the exported sidebar order, and the search and Markdown outputs. See [Testing](content/docs/testing.md) for server checks and manual live-service gates.
