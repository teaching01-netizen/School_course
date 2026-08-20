import type { StorybookConfig } from "@storybook/react-vite";
import path from "node:path";
import tailwindcss from "@tailwindcss/vite";

const config: StorybookConfig = {
  stories: ["../src/**/*.stories.@(ts|tsx)"],
  framework: { name: "@storybook/react-vite", options: {} },
  viteFinal: async (viteConfig) => {
    viteConfig.plugins = [...(viteConfig.plugins ?? []), tailwindcss()];
    viteConfig.resolve = {
      ...viteConfig.resolve,
      alias: { ...viteConfig.resolve?.alias, "@": path.resolve(import.meta.dirname, "../src") },
    };
    return viteConfig;
  },
};

export default config;
