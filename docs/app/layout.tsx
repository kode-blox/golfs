import { Provider } from "@/components/provider";
import { siteBasePath, siteOrigin } from "@/lib/shared";
import type { Metadata } from "next";
import type { ReactNode } from "react";
import "./global.css";

export const metadata: Metadata = {
  metadataBase: new URL(`${siteOrigin}${siteBasePath || "/"}`),
  title: {
    default: "GOLFS Docs",
    template: "%s | GOLFS Docs",
  },
  description: "Documentation for the GOLFS Git LFS server.",
};

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="flex min-h-screen flex-col">
        <Provider>{children}</Provider>
      </body>
    </html>
  );
}
