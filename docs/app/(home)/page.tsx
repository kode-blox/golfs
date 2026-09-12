import Link from "next/link";
import Image from "next/image";

export default function HomePage() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center px-6 py-24 text-center">
      <div className="mx-auto flex max-w-3xl flex-col items-center gap-6">
        <Image src="/images/logo.svg" alt="" width={64} height={64} unoptimized className="size-16" />
        <div className="space-y-3">
          <p className="text-sm font-medium text-fd-muted-foreground">Git LFS infrastructure</p>
          <h1 className="text-4xl font-bold tracking-tight sm:text-6xl">GOLFS Docs</h1>
          <p className="mx-auto max-w-2xl text-balance text-lg text-fd-muted-foreground">
            Architecture, configuration, client setup, operations, security, and release guidance for the GOLFS
            stateless Git LFS server.
          </p>
        </div>
        <div className="flex flex-wrap justify-center gap-3">
          <Link
            href="/architecture"
            className="inline-flex h-10 items-center rounded-md bg-fd-primary px-4 text-sm font-medium text-fd-primary-foreground transition-colors hover:bg-fd-primary/90"
          >
            Read the architecture
          </Link>
          <Link
            href="/github-app-and-gcm"
            className="inline-flex h-10 items-center rounded-md border px-4 text-sm font-medium transition-colors hover:bg-fd-accent hover:text-fd-accent-foreground"
          >
            Configure a client
          </Link>
        </div>
      </div>
    </div>
  );
}
