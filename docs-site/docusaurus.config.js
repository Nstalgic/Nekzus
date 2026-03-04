// @ts-check
import { themes as prismThemes } from "prism-react-renderer";
import { remarkKroki } from "remark-kroki";

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: "Nekzus",
  tagline:
    "Secure API gateway for homelab with auto-discovery, reverse proxy, and web dashboard",
  favicon: "img/favicon.ico",

  url: "https://nstalgic.github.io",
  baseUrl: "/Nekzus/",

  organizationName: "nstalgic",
  projectName: "nekzus",

  onBrokenLinks: "throw",

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: "warn",
    },
  },

  themes: ["@docusaurus/theme-mermaid"],

  presets: [
    [
      "classic",
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          routeBasePath: "/",
          sidebarPath: "./sidebars.js",
          editUrl: "https://github.com/nstalgic/nekzus/edit/main/docs-site/",
          remarkPlugins: [
            [
              remarkKroki,
              {
                server: "https://kroki.io",
                alias: ["d2"],
                target: "mdx3",
              },
            ],
          ],
        },
        blog: false,
        theme: {
          customCss: "./src/css/custom.css",
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      colorMode: {
        defaultMode: "dark",
        respectPrefersColorScheme: true,
      },
      navbar: {
        title: "Nekzus",
        logo: {
          alt: "Nekzus Logo",
          src: "img/logo.png",
        },
        items: [
          {
            type: "docSidebar",
            sidebarId: "docs",
            position: "left",
            label: "Documentation",
          },
          {
            href: "https://github.com/nstalgic/nekzus",
            label: "GitHub",
            position: "right",
          },
        ],
      },
      footer: {
        style: "dark",
        links: [
          {
            title: "Docs",
            items: [
              { label: "Getting Started", to: "/getting-started/installation" },
              { label: "Configuration", to: "/getting-started/configuration" },
              { label: "API Reference", to: "/reference/api" },
            ],
          },
          {
            title: "Community",
            items: [
              {
                label: "GitHub",
                href: "https://github.com/nstalgic/nekzus",
              },
              {
                label: "Docker Hub",
                href: "https://hub.docker.com/r/nstalgic/nekzus",
              },
              {
                label: "Issues",
                href: "https://github.com/nstalgic/nekzus/issues",
              },
            ],
          },
        ],
        copyright: `Copyright \u00a9 ${new Date().getFullYear()} Nekzus. Built with Docusaurus.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ["bash", "json", "yaml", "go", "toml"],
      },
    }),
};

export default config;
