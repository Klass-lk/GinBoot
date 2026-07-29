import defaultMdxComponents from 'fumadocs-ui/mdx';
import type { MDXComponents } from 'mdx/types';
import type { ComponentProps } from 'react';
import { Accordion, Accordions } from 'fumadocs-ui/components/accordion';
import { Callout } from 'fumadocs-ui/components/callout';
import { Card, Cards } from 'fumadocs-ui/components/card';
import { File, Files, Folder } from 'fumadocs-ui/components/files';
import { ImageZoom, type ImageZoomProps } from 'fumadocs-ui/components/image-zoom';
import { Step, Steps } from 'fumadocs-ui/components/steps';
import { Tab, Tabs } from 'fumadocs-ui/components/tabs';
import { TypeTable } from 'fumadocs-ui/components/type-table';
import { cn } from '@/lib/cn';
import { ApiSignature } from '@/components/mdx/api-signature';
import { Since } from '@/components/mdx/since';

export function getMDXComponents(components?: MDXComponents) {
  return {
    ...defaultMdxComponents,
    // fumadocs ships these but doesn't register them by default; without this,
    // MDX authors would have to import each one in every file.
    Accordion,
    Accordions,
    Callout,
    Card,
    Cards,
    File,
    Files,
    Folder,
    Step,
    Steps,
    Tab,
    Tabs,
    TypeTable,
    // Ginboot-specific
    ApiSignature,
    Since,
    // The docs screenshots are dense; make them zoomable. The cast mirrors what
    // fumadocs' own default `img` does — MDX types `src` as `string | Blob`,
    // while next/image wants `string | StaticImport`. Markdown only ever emits
    // a string.
    img: (props: ComponentProps<'img'>) => (
      <ImageZoom
        sizes="(max-width: 768px) 100vw, (max-width: 1200px) 70vw, 900px"
        {...(props as ImageZoomProps)}
        className={cn('rounded-lg', props.className)}
      />
    ),
    ...components,
  } satisfies MDXComponents;
}

export const useMDXComponents = getMDXComponents;

declare global {
  type MDXProvidedComponents = ReturnType<typeof getMDXComponents>;
}
