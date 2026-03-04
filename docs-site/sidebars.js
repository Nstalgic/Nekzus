/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    {
      type: "doc",
      id: "index",
      label: "Home",
    },
    {
      type: "category",
      label: "Getting Started",
      items: [
        "getting-started/installation",
        "getting-started/quick-start",
        "getting-started/configuration",
      ],
    },
    {
      type: "category",
      label: "Guides",
      items: [
        "guides/docker-compose",
        "guides/upgrade",
        "guides/troubleshooting",
      ],
    },
    {
      type: "category",
      label: "Platforms",
      items: [
        "platforms/index",
        "platforms/synology",
        "platforms/unraid",
        "platforms/proxmox",
        "platforms/raspberry-pi",
      ],
    },
    {
      type: "category",
      label: "Features",
      items: [
        "features/web-dashboard",
        "features/discovery",
        "features/toolbox",
        "features/notifications",
        "features/scripts",
      ],
    },
    {
      type: "category",
      label: "Reference",
      items: [
        "reference/api",
        "reference/api-containers",
        "reference/cli",
        "reference/configuration",
      ],
    },
    {
      type: "category",
      label: "Development",
      items: [
        "development/contributing",
        "development/testing",
        "development/architecture",
      ],
    },
    {
      type: "category",
      label: "Kubernetes",
      items: ["kubernetes/index", "kubernetes/rollout"],
    },
  ],
};

export default sidebars;
