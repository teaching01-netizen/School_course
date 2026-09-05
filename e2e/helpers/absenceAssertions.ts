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
    .locator('input[aria-label="Confirmation code"]')
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
        // The shell scrolls the focused control into the strip between the
        // header and the action bar. A control shorter than the strip must be
        // fully contained; a taller one (e.g. the reason field with the
        // keyboard open) is anchored against an edge of the strip — its top
        // at the strip start when there is room above it, otherwise its
        // bottom just above the action bar — which is the most that can be
        // brought into view.
        const stripTop = mainRect.top;
        const stripBottom = visibleBottom;
        const fits = rect.height <= stripBottom - stripTop + 2;
        const topReachable = rect.top >= stripTop - 1 && rect.top <= stripBottom + 1;
        const bottomReachable = rect.bottom >= stripTop - 1 && rect.bottom <= stripBottom + 1;
        const spansStrip = rect.top <= stripTop && rect.bottom >= stripBottom;
        return fits
          ? rect.top >= stripTop - 1 && rect.bottom <= stripBottom + 1
          : topReachable || bottomReachable || spansStrip;
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

export async function expectProgress(page: Page, minPercent: number, maxPercent: number) {
  const progress = page.getByRole("progressbar");
  await expect(progress).toBeVisible();
  const value = Number(await progress.getAttribute("aria-valuenow"));
  expect(value).toBeGreaterThanOrEqual(minPercent);
  expect(value).toBeLessThanOrEqual(maxPercent);
}
