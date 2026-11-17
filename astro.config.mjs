import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { unified } from '@astrojs/markdown-remark';

const base = '/terraview';

/**
 * Starlight routes are extensionless, lowercased and base-prefixed.
 * Rewrite local Markdown links (page.md, page.md#section) to generated routes.
 */
function remarkDocsLinks() {
  return (tree) => {
    const walk = (node) => {
      if (node.type === 'link' && typeof node.url === 'string') {
        const href = node.url;
        if (!/^(https?:|mailto:|#|\/)/.test(href)) {
          const [target, hash] = href.split('#');
          if (target.endsWith('.md')) {
            let slug = target.replace(/^\.\//, '').replace(/\.md$/, '').toLowerCase();
            if (slug === 'index') slug = 'overview';
            node.url = `${base}/${slug}/${hash ? `#${hash}` : ''}`;
          }
        }
      }
      if (Array.isArray(node.children)) node.children.forEach(walk);
    };
    walk(tree);
  };
}

export default defineConfig({
  site: 'https://slimaineb.github.io',
  base,
  trailingSlash: 'always',
  outDir: './site',
  markdown: {
    processor: unified({ remarkPlugins: [remarkDocsLinks] }),
  },
  integrations: [
    starlight({
      title: 'TerraView docs',
      disable404Route: true,
      description:
        'Turn Terraform plan JSON into an interactive dependency graph with diffs and Atlantis commands. One binary. Any provider.',
      logo: {
        src: './src/assets/logo.svg',
        alt: 'TerraView',
      },
      favicon: '/favicon.svg',
      customCss: ['./src/styles/docs.css'],
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/SlimaineB/terraview',
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/SlimaineB/terraview/edit/main/docs/',
      },
      tableOfContents: { minHeadingLevel: 2, maxHeadingLevel: 3 },
      sidebar: [
        {
          label: 'Start here',
          items: [
            { label: 'Overview', slug: 'overview' },
            { label: 'Quick start', slug: 'install' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'Install', slug: 'install' },
            { label: 'Usage', slug: 'usage' },
            { label: 'UI guide', slug: 'ui' },
          ],
        },
        {
          label: 'Integrations',
          items: [
            { label: 'CI reports', slug: 'ci' },
          ],
        },
        {
          label: 'Develop',
          items: [
            { label: 'Development', slug: 'development' },
          ],
        },
      ],
    }),
  ],
});