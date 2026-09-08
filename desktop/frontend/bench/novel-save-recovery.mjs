import assert from "node:assert/strict";
import { chromium } from "playwright";

const baseURL = process.env.REASONIX_UI_REVIEW_URL ?? "http://127.0.0.1:5187";
const browser = await chromium.launch({ channel: process.env.REASONIX_BROWSER_CHANNEL || undefined, headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, locale: "zh-CN" });
  const errors = [];
  page.on("pageerror", (error) => errors.push(String(error)));
  await page.goto(`${baseURL}/?mock=fresh`, { waitUntil: "domcontentloaded" });
  await page.locator(".onboarding__skip").click();
  await page.evaluate(async () => {
    const { app } = await import("/src/lib/bridge.ts");
    const load = app.LoadNovelProject;
    const overrides = {
      LoadNovelProject: async () => {
        const project = await load();
        return { ...project, chapters: [...project.chapters, { ...project.chapters[0], id: "chapter-2", title: "Second chapter", content: "second" }] };
      },
      SaveNovelChapter: (projectId, chapter) => {
        window.novelWrites.push({ projectId, chapter });
        return new Promise((resolve, reject) => { window.novelPending = { resolve: () => resolve(chapter), reject: () => reject(new Error("disk full")) }; });
      },
    };
    window.novelWrites = [];
    window.go = { main: { App: new Proxy(overrides, {
      get(target, prop) {
        if (prop in target) return target[prop];
        const go = window.go;
        delete window.go;
        const fallback = app[prop];
        window.go = go;
        return fallback;
      },
    }) } };
  });
  await page.locator(".workbench-launch--violet").click();
  const editor = page.locator(".novel-editor textarea");
  await editor.waitFor();
  await editor.fill("A");
  await page.locator(".novel-editor__toolbar button").click();
  await page.waitForFunction(() => window.novelWrites.length === 1);
  await editor.fill("B - newest edit");
  await page.evaluate(() => window.novelPending.resolve());
  await page.waitForFunction(() => window.novelWrites.length === 2);
  assert.equal(await editor.inputValue(), "B - newest edit", "stale save cannot overwrite new edits");
  assert.equal(await page.evaluate(() => window.novelWrites[1].chapter.content), "B - newest edit");
  await page.evaluate(() => window.novelPending.resolve());

  await editor.fill("C - chapter switch");
  await page.locator(".novel-writing__chapters > button").nth(1).click();
  await page.waitForFunction(() => window.novelWrites.length === 3);
  assert.equal(await editor.inputValue(), "C - chapter switch", "chapter navigation waits for save");
  await page.evaluate(() => window.novelPending.resolve());
  await page.waitForFunction(() => document.querySelector(".novel-editor textarea")?.value === "second");

  await editor.fill("D - close recovery");
  await page.locator(".novel-studio__close").click();
  await page.waitForFunction(() => window.novelWrites.length === 4);
  assert(await page.locator(".novel-studio").isVisible(), "close waits for pending save");
  await page.evaluate(() => window.novelPending.reject());
  await page.getByText(/保存失败/).waitFor();
  assert.equal(await editor.inputValue(), "D - close recovery", "failed save preserves editable draft");
  await page.locator(".novel-studio__close").click();
  await page.waitForFunction(() => window.novelWrites.length === 5);
  await page.evaluate(() => window.novelPending.resolve());
  await page.locator(".novel-studio").waitFor({ state: "detached" });

  await page.locator(".workbench-launch--violet").click();
  await editor.waitFor();
  await editor.fill("E - sidebar navigation");
  await page.locator(".sidebar__quick-action").filter({ hasText: "PPT 智能制作" }).click();
  await page.waitForFunction(() => window.novelWrites.length === 6);
  assert(await page.locator(".novel-studio").isVisible(), "sidebar navigation also waits for save");
  await page.evaluate(() => window.novelPending.resolve());
  await page.locator(".novel-studio").waitFor({ state: "detached" });
  await page.locator(".creation-center").waitFor();
  await page.locator(".creation-center__close").click();
  await page.locator(".workbench-launch--violet").click();
  await editor.waitFor();
  await page.evaluate(() => {
    window.reviewWindowCloseCount = 0;
    window.go.main.App.CloseMainWindow = async () => { window.reviewWindowCloseCount += 1; };
  });
  await editor.fill("F - window close");
  await page.locator(".windows-window-control--close").click();
  await page.waitForFunction(() => window.novelWrites.length === 7);
  assert.equal(await page.evaluate(() => window.reviewWindowCloseCount), 0, "window close waits for disk write");
  await page.evaluate(() => window.novelPending.resolve());
  await page.waitForFunction(() => window.reviewWindowCloseCount === 1);
  assert.deepEqual(errors, [], "no runtime errors");
  console.log("PASS novel delayed response, chapter switching, failed-save retry, studio/window close and sidebar navigation");
} finally { await browser.close(); }
