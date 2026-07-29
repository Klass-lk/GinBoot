/**
 * Marks when an API landed, e.g. `<Since v="1.1.0" />`. Keep the versions in
 * sync with `content/docs/4-advanced/changelog.mdx`.
 */
export function Since({ v }: { v: string }) {
  return (
    <span className="not-prose ml-2 inline-flex items-center rounded-full bg-brand/10 px-2 py-0.5 align-middle font-mono text-[11px] font-semibold text-brand ring-1 ring-brand/20">
      v{v}
    </span>
  );
}
