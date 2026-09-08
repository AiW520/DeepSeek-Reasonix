import assert from "node:assert/strict";
import { createNovelChapterSaves } from "../lib/novelChapterSaves";
import type { NovelChapter } from "../lib/types";

const chapter = (content: string, id = "chapter-1"): NovelChapter => ({
  id, content, title: "Chapter", outline: "", summary: "", status: "draft", order: 1, wordCount: content.length, updatedAt: 0,
});
const tick = async () => { for (let i = 0; i < 5; i++) await Promise.resolve(); };
const writes: Array<{ project: string; chapter: NovelChapter; resolve: (saved: NovelChapter) => void; reject: (error: Error) => void }> = [];
const applied: string[] = [];
const saves = createNovelChapterSaves((project, chapter) => new Promise((resolve, reject) => {
  writes.push({ project, chapter, resolve, reject });
}), (_project, _submitted, saved) => applied.push(saved.content));

saves.edit("project-1", chapter("A"));
const first = saves.flush();
assert.equal(saves.flush(), first, "manual save and autosave share the same writer");
await tick();
saves.edit("project-1", chapter("B"));
saves.edit("project-1", chapter("C"));
assert.equal(writes.length, 1, "new edits cannot start an overlapping write");
writes[0].resolve(writes[0].chapter);
await tick();
assert.deepEqual(applied, [], "old responses never replace newer draft content");
assert.equal(writes[1].chapter.content, "C", "intermediate keystrokes coalesce to the latest revision");
writes[1].resolve(writes[1].chapter);
await first;
assert.deepEqual(applied, ["C"]);
assert.equal(saves.hasPending(), false);

saves.edit("project-1", chapter("D"));
const failed = saves.flush();
await tick();
writes[2].reject(new Error("disk full"));
await assert.rejects(failed, /disk full/);
assert(saves.hasPending(), "failed writes retain the draft for retry and block navigation");
const retry = saves.flush();
await tick();
writes[3].resolve(writes[3].chapter);
await retry;
assert.equal(saves.hasPending(), false);

saves.edit("project-1", chapter("one"));
saves.edit("project-2", chapter("two"));
const close = saves.flush();
await tick();
assert.equal(writes[4].project, "project-1");
writes[4].resolve(writes[4].chapter);
await tick();
assert.equal(writes[5].project, "project-2", "identical chapter IDs in different projects stay isolated");
writes[5].resolve(writes[5].chapter);
await close;
assert.deepEqual(applied, ["C", "D", "one", "two"]);
console.log("PASS novel save ordering, revision coalescing, stale response suppression, retry and project isolation");
