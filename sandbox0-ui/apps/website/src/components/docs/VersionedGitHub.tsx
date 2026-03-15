"use client";

import { usePathname } from "next/navigation";
import { PixelCodeBlock } from "./PixelCodeBlock";
import { DocsLink } from "./DocsLink";
import {
  getResolvedDocsVersionFromPathname,
  toGitHubRawHref,
} from "./versioning";

type RepoName = "sandbox0";

function repoSlug(repo: RepoName): string {
  return `sandbox0-ai/${repo}`;
}

export function GitHubRawLink({
  repo,
  path,
  children,
}: {
  repo: RepoName;
  path: string;
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const version = getResolvedDocsVersionFromPathname(pathname);
  const href = toGitHubRawHref(repoSlug(repo), version, path);

  return (
    <DocsLink href={href} newTab>
      {children}
    </DocsLink>
  );
}

export function GitHubApplyCommand({
  repo,
  path,
}: {
  repo: RepoName;
  path: string;
}) {
  const pathname = usePathname();
  const version = getResolvedDocsVersionFromPathname(pathname);
  const href = toGitHubRawHref(repoSlug(repo), version, path);

  return (
    <PixelCodeBlock language="bash" scale="md" className="mb-6">
      {`kubectl apply -f ${href}`}
    </PixelCodeBlock>
  );
}
