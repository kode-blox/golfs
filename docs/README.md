# GOLFS documentation site

The documentation website uses the official `fumadocs-mdx` content source, Fumadocs Core and UI, and a statically exported Next.js application. Authored documentation lives in `content/docs`.

The landing page and documentation pages use Fumadocs layouts, documentation pages expose copy and source-view controls, and the static export includes search, Open Graph images, and machine-readable Markdown routes.

From the repository root:

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
