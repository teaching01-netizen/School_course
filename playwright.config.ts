import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  fullyParallel: false,
  reporter: [["list"]],
  use: {
    baseURL: "http://127.0.0.1:4173",
    trace: "on-first-retry",
  },
  webServer: {
    command: "npm run build && npm run preview -- --host 127.0.0.1 --port 4173",
    url: "http://127.0.0.1:4173",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
  projects: [
    {
      name: "chromium-phone-small",
      use: { ...devices["Pixel 5"], viewport: { width: 320, height: 844 } },
    },
    {
      name: "chromium-phone-compact",
      use: { ...devices["Pixel 5"], viewport: { width: 360, height: 800 } },
    },
    {
      name: "chromium-phone-standard",
      use: { ...devices["Pixel 7"], viewport: { width: 390, height: 844 } },
    },
    {
      name: "chromium-phone-large",
      use: { ...devices["Pixel 7"], viewport: { width: 412, height: 915 } },
    },
    {
      name: "chromium-phone-landscape",
      use: { ...devices["Pixel 7"], viewport: { width: 844, height: 390 } },
    },
    {
      name: "chromium-tablet-portrait",
      use: { ...devices["Desktop Chrome"], viewport: { width: 768, height: 1024 } },
    },
    {
      name: "chromium-tablet-landscape",
      use: { ...devices["Desktop Chrome"], viewport: { width: 1024, height: 768 } },
    },
    {
      name: "webkit-phone",
      use: { ...devices["iPhone 13"], viewport: { width: 390, height: 844 } },
    },
    {
      name: "webkit-tablet-portrait",
      use: { ...devices["iPad (gen 11)"], viewport: { width: 768, height: 1024 } },
    },
    {
      name: "webkit-tablet-landscape",
      use: { ...devices["iPad (gen 11) landscape"], viewport: { width: 1024, height: 768 } },
    },
    {
      name: "webkit-tablet-large",
      use: { ...devices["iPad (gen 11)"], viewport: { width: 1024, height: 1366 } },
    },
    {
      name: "firefox-phone",
      use: { ...devices["Desktop Firefox"], viewport: { width: 390, height: 844 } },
    },
    {
      name: "chromium-desktop",
      use: { ...devices["Desktop Chrome"], viewport: { width: 1440, height: 900 } },
    },
  ],
});
