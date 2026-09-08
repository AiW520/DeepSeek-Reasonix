import assert from "node:assert/strict";
import { mkdir } from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";

// Run against the frontend dev mock; no provider credentials or paid calls.
const baseURL = process.env.REASONIX_UI_REVIEW_URL ?? "http://127.0.0.1:5187";
const output = process.env.REASONIX_UI_REVIEW_OUTPUT;
if (output) await mkdir(output, { recursive: true });
const browser = await chromium.launch({ channel: process.env.REASONIX_BROWSER_CHANNEL || undefined, headless: true });
const errors = [];
const screenshot = async (page, name) => {
  if (output) await page.screenshot({ path: path.join(output, `${name}.png`) });
};

try {
  for (const [width, height] of [[1440, 1000], [1024, 768], [760, 480], [390, 844]]) {
    const page = await browser.newPage({ viewport: { width, height }, locale: "zh-CN" });
    page.on("pageerror", (error) => errors.push(String(error)));
    await page.goto(`${baseURL}/?mock=fresh&platform=windows`, { waitUntil: "domcontentloaded" });
    await page.locator(".onboarding__input").waitFor();
    await page.locator(".onboarding__input").focus();
    await page.keyboard.press("Shift+Tab");
    assert(await page.locator(".onboarding__skip").evaluate((el) => el === document.activeElement), "backward focus stays in onboarding");
    await page.keyboard.press("Tab");
    assert(await page.locator(".onboarding__input").evaluate((el) => el === document.activeElement), "forward focus wraps to key input");
    await page.locator(".onboarding__submit").click();
    assert.equal(await page.locator(".onboarding__input").getAttribute("aria-invalid"), "true");
    assert(await page.locator("#onboarding-error").isVisible());
    await page.locator(".onboarding__skip").scrollIntoViewIfNeeded();
    const skip = await page.locator(".onboarding__skip").boundingBox();
    assert(skip.y >= -1 && skip.y + skip.height <= height + 1, "skip remains reachable in short windows");
    await screenshot(page, `onboarding-${width}`);
    await page.locator(".onboarding__skip").click();
    await page.locator(".workbench-launch").first().waitFor();
    await page.locator(".workbench-launch").last().scrollIntoViewIfNeeded();
    const overflow = await page.locator(".workbench-launch").evaluateAll((cards) => cards.flatMap((card) => {
      const outer = card.getBoundingClientRect();
      return [...card.querySelectorAll("strong, em, .workbench-launch__meta")].filter((el) => {
        const rect = el.getBoundingClientRect();
        return rect.left < outer.left - 1 || rect.right > outer.right + 1 || rect.bottom > outer.bottom + 1;
      }).map((el) => el.textContent);
    }));
    assert.deepEqual(overflow, [], `card content fits at ${width}px`);
    await screenshot(page, `workbench-${width}`);
    if (width >= 760) {
      await page.locator(".workbench-home__model-settings").click();
      await page.locator(".settings-modal").waitFor();
      await screenshot(page, `settings-${width}`);
    }
    console.log(`PASS onboarding focus, validation, scrolling and workbench layout: ${width}x${height}`);
    await page.close();
  }

  const page = await browser.newPage({ viewport: { width: 1024, height: 768 }, locale: "zh-CN" });
  page.on("pageerror", (error) => errors.push(String(error)));
  await page.goto(`${baseURL}/?mock=fresh`, { waitUntil: "domcontentloaded" });
  await page.locator(".onboarding__input").fill("local-test-value");
  await page.evaluate(async () => {
    window.reviewConnectCalls = 0;
    const connect = () => {
      window.reviewConnectCalls += 1;
      return new Promise((_, reject) => { window.reviewRejectConnect = () => reject(new Error("network timeout")); });
    };
    window.go = { main: { App: { ConnectKey: connect } } };
    const submit = document.querySelector(".onboarding__submit");
    submit.click();
    submit.click();
  });
  assert.equal(await page.evaluate(() => window.reviewConnectCalls), 1, "duplicate submissions make one request");
  await page.keyboard.press("Escape");
  assert(await page.locator(".onboarding").isVisible(), "pending setup cannot be dismissed");
  await page.evaluate(() => window.reviewRejectConnect());
  await page.locator("#onboarding-error").waitFor();
  assert(await page.locator(".onboarding__input").evaluate((el) => el === document.activeElement && el.selectionEnd === el.value.length), "failed setup restores editable key focus");
  await page.keyboard.press("Escape");
  await page.locator(".onboarding").waitFor({ state: "detached" });
  console.log("PASS duplicate request guard, failed-request recovery and Escape dismissal");
  assert.deepEqual(errors, [], "no browser runtime errors");
} finally {
  await browser.close();
}
