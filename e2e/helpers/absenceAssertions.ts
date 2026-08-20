import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";

export async function expectAccessiblePage(page: Page) {
  await expectAnimationsSettled(page);
  const results = await new AxeBuilder({ page }).analyze();
  const blocking = results.violations.filter(
    (violation) => violation.impact === "critical" || violation.impact === "serious",
  );
  expect(blocking, blocking.map(({ id, help }) => `${id}: ${help}`).join("\n")).toEqual([]);
}

export async function expectNoHorizontalOverflow(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
}

export async function expectAnimationsSettled(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() => {
        const fadingElements = [...document.querySelectorAll<HTMLElement>('[style*="opacity"]')].filter((element) => {
          const opacity = Number.parseFloat(getComputedStyle(element).opacity);
          return element.getClientRects().length > 0 && opacity < 0.999;
        }).length;
        const runningAnimations = document.getAnimations().filter((animation) => {
          const timing = animation.effect?.getTiming();
          const iterations = timing?.iterations;
          const duration = timing?.duration;
          // WebKit reports reduced-motion's 0.01ms transitions as running at time 0.
          return animation.playState === "running"
            && iterations !== Infinity
            && (typeof duration !== "number" || duration > 1);
        }).length;
        return fadingElements + runningAnimations;
      }),
    )
    .toBe(0);
}

export async function expectOtpBoxesInsideViewport(page: Page) {
  const boxes = page
    .locator('input[aria-label="Verification code"]')
    .locator("xpath=..")
    .locator(':scope > div[aria-hidden="true"]');
  await expect(boxes).toHaveCount(6);
  const viewportWidth = await page.evaluate(() => window.innerWidth);
  for (const box of await boxes.all()) {
    const bounds = await box.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewportWidth);
    expect(Math.abs(bounds!.width - bounds!.height)).toBeLessThanOrEqual(1);
    expect(bounds!.width).toBeLessThanOrEqual(viewportWidth / 6);
  }
}

export async function expectInsideVisualViewport(page: Page, selector: string) {
  await expect
    .poll(() =>
      page.evaluate((targetSelector) => {
        const target = document.querySelector<HTMLElement>(targetSelector);
        if (!target) return false;
        const rect = target.getBoundingClientRect();
        const visualViewport = window.visualViewport;
        const top = visualViewport?.offsetTop ?? 0;
        const right = visualViewport?.width ?? window.innerWidth;
        const bottom = top + (visualViewport?.height ?? window.innerHeight);
        return rect.left >= -1 && rect.right <= right + 1 && rect.top >= top - 1 && rect.bottom <= bottom + 1;
      }, selector),
    )
    .toBe(true);
}

export async function expectFocusedElementVisible(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() => {
        const active = document.activeElement;
        const main = document.querySelector<HTMLElement>(".absence-app-shell__main");
        const footer = document.querySelector<HTMLElement>(".absence-app-shell__footer");
        if (!(active instanceof HTMLElement) || !main || !main.contains(active)) return false;
        const rect = active.getBoundingClientRect();
        const mainRect = main.getBoundingClientRect();
        const footerRect = footer?.getBoundingClientRect();
        const visibleBottom = Math.min(mainRect.bottom, footerRect?.top ?? mainRect.bottom);
        return rect.top >= mainRect.top - 1 && rect.bottom <= visibleBottom + 1;
      }),
    )
    .toBe(true);
}

export async function expectNoActionOverlap(page: Page) {
  await expect
    .poll(() =>
      page.evaluate(() => {
        const main = document.querySelector<HTMLElement>(".absence-app-shell__main");
        const footer = document.querySelector<HTMLElement>(".absence-app-shell__footer");
        const shell = document.querySelector<HTMLElement>(".absence-app-shell");
        if (!main || !footer || !shell) return false;
        const mainRect = main.getBoundingClientRect();
        const footerRect = footer.getBoundingClientRect();
        const shellRect = shell.getBoundingClientRect();
        return footerRect.top >= mainRect.bottom - 1
          && footerRect.bottom <= shellRect.bottom + 1
          && footerRect.left >= shellRect.left - 1
          && footerRect.right <= shellRect.right + 1;
      }),
    )
    .toBe(true);
}

export async function expectMinimumTapTarget(page: Page, selector: string) {
  const target = page.locator(selector);
  await expect(target).toBeVisible();
  const bounds = await target.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.width).toBeGreaterThanOrEqual(44);
  expect(bounds!.height).toBeGreaterThanOrEqual(44);
}

export async function expectCurrentStep(page: Page, step: number, label: string) {
  const progress = page.getByRole("navigation", { name: "Progress" });
  const currentStepButton = progress.getByRole("button", {
    name: new RegExp(`${label}.*current`, "i"),
  });
  await expect(currentStepButton).toBeVisible();
  await expect(currentStepButton).toContainText(String(step));
}

export async function expectAdaptiveTabletClassesLayout(page: Page) {
  await expect(page.locator(".absence-classes-layout")).toBeVisible();
  const isLandscapeTablet = await page.evaluate(
    () => window.innerWidth >= 960 && window.innerWidth > window.innerHeight,
  );
  if (!isLandscapeTablet) return;

  const subjects = await page.locator(".absence-classes-layout__subjects").boundingBox();
  const work = await page.locator(".absence-classes-layout__work").boundingBox();
  expect(subjects).not.toBeNull();
  expect(work).not.toBeNull();
  expect(work!.x).toBeGreaterThan(subjects!.x + 1);
  expect(Math.abs(work!.y - subjects!.y)).toBeLessThanOrEqual(40);
}
