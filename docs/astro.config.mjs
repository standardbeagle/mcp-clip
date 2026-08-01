import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://dev.standardbeagle.com',
  base: '/mcp-clip',
  integrations: [
    starlight({
      title: 'mcp-clip',
      description: 'A high-performance, lock-free MCP server that gives AI assistants clipboard access, with WSL2 support for the VSCode image-paste gap.',
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/standardbeagle/mcp-clip' },
      ],
      head: [
        {
          tag: 'meta',
          attrs: { property: 'og:title', content: 'mcp-clip' },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:description', content: 'A high-performance, lock-free MCP server that gives AI assistants clipboard access, with WSL2 support for the VSCode image-paste gap.' },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:type', content: 'website' },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:url', content: 'https://dev.standardbeagle.com/mcp-clip/' },
        },
        {
          tag: 'meta',
          attrs: { name: 'twitter:card', content: 'summary' },
        },
      ],
      customCss: ['./src/styles/custom.css'],
      sidebar: [
        {
          label: 'Start Here',
          items: [
            { label: 'Getting Started', slug: 'getting-started' },
          ],
        },
        {
          label: 'Tools',
          autogenerate: { directory: 'tools' },
        },
        {
          label: 'Guides',
          items: [
            { label: 'WSL2 Setup', slug: 'guides/wsl2' },
          ],
        },
      ],
    }),
  ],
});
