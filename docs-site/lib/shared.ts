export const appName = 'Ginboot';
export const appTagline = 'Spring Boot, for Go.';
export const siteUrl = 'https://ginboot.com';

export const docsRoute = '/docs';
export const docsImageRoute = '/og/docs';
export const docsContentRoute = '/llms.mdx/docs';

export const gitConfig = {
  user: 'klass-lk',
  repo: 'ginboot',
  branch: 'main',
};

export const externalLinks = {
  github: `https://github.com/${gitConfig.user}/${gitConfig.repo}`,
  initializer: 'https://start.ginboot.com',
  cloud: 'https://cloud.ginboot.com',
};

// The docs site lives in the `docs-site/` subdirectory of the repo, so the
// content path fumadocs reports (`page.path`) needs prefixing to resolve to a
// real file on GitHub.
const contentRootInRepo = 'docs-site/content/docs';

export function getEditUrl(pagePath: string) {
  const { user, repo, branch } = gitConfig;

  return `https://github.com/${user}/${repo}/blob/${branch}/${contentRootInRepo}/${pagePath}`;
}
