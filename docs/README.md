# GOLFS documentation site

The documentation website uses the official `fumadocs-mdx` content source, Fumadocs Core and UI, and a statically exported Next.js application. Authored documentation lives in `content/docs`.

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

GitHub Actions sets `NEXT_PUBLIC_BASE_PATH=/golfs` for the project Pages site. Set the same variable locally when testing Pages-prefixed URLs; do not hard-code the repository prefix into authored Markdown links.
