import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'http://127.0.0.1:4332',
  server: {
    host: '127.0.0.1',
    port: 4332,
  },
  devToolbar: { enabled: false },
  vite: {
    // Node's ESM resolver can pick a cookie build without named parseCookie
    // when the prerender entry leaves it external. Bundle it.
    ssr: { noExternal: ['cookie'] },
  },
  integrations: [
    starlight({
      title: 'Nicos Catalog',
      description:
        'User docs for the portable Nicos Catalog engine: Layout, providers, CLI, and the closed public DTO.',
      logo: {
        src: './public/favicon.svg',
        alt: 'Nicos Catalog',
      },
      favicon: '/favicon.svg',
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/nstranquist/nicos-catalog',
        },
      ],
      sidebar: [
        {
          label: 'Start',
          items: [
            { label: 'Docs', slug: 'docs' },
            { label: 'Install', slug: 'install' },
          ],
        },
        {
          label: 'Use the engine',
          items: [
            { label: 'Host contract', slug: 'host' },
            { label: 'Search', slug: 'search' },
            { label: 'Graph', slug: 'graph' },
            { label: 'Drift and reconcile', slug: 'drift' },
            { label: 'CLI', slug: 'cli' },
          ],
        },
        {
          label: 'Contracts',
          items: [
            { label: 'Architecture', slug: 'architecture' },
            { label: 'API stability', slug: 'stability' },
            { label: 'Privacy and public DTO', slug: 'privacy' },
            { label: 'Migrate from v0.1', slug: 'migrate' },
          ],
        },
        {
          label: 'Project',
          items: [
            { label: 'Contribute', slug: 'contribute' },
          ],
        },
      ],
      customCss: ['./src/styles/nicos.css'],
      pagination: true,
      tableOfContents: {
        minHeadingLevel: 2,
        maxHeadingLevel: 3,
      },
      lastUpdated: false,
    }),
  ],
});
