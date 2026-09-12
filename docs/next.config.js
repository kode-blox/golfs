import { createMDX } from "fumadocs-mdx/next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants.js";

const withMDX = createMDX();

function normalizeBasePath(value) {
  if (!value) return "";

  const withLeadingSlash = value.startsWith("/") ? value : `/${value}`;
  return withLeadingSlash.replace(/\/$/, "");
}

export default function config(phase) {
  const isDevelopmentServer = phase === PHASE_DEVELOPMENT_SERVER;
  const basePath = normalizeBasePath(process.env.NEXT_PUBLIC_BASE_PATH);

  return withMDX({
    reactStrictMode: true,
    agentRules: false,
    basePath,
    trailingSlash: true,
    ...(isDevelopmentServer ? {} : { output: "export" }),
  });
}
