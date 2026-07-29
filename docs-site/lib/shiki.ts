/**
 * Single source of truth for code highlighting, shared by MDX (via
 * `source.config.ts`) and the landing page's `DynamicCodeBlock`, so inline docs
 * code and marketing code never drift apart.
 */
export const shikiThemes = {
  light: 'github-light',
  dark: 'github-dark-default',
} as const;
