import type { NovelChapter } from "./types";

type Draft = { projectId: string; chapter: NovelChapter };

/** One writer drains the latest revision of each chapter, retaining failed drafts. */
export function createNovelChapterSaves(
  write: (projectId: string, chapter: NovelChapter) => Promise<NovelChapter>,
  onSaved: (projectId: string, submitted: NovelChapter, saved: NovelChapter) => void,
) {
  const drafts = new Map<string, Draft>();
  let running: Promise<void> | null = null;
  const key = (projectId: string, chapterId: string) => JSON.stringify([projectId, chapterId]);

  return {
    edit(projectId: string, chapter: NovelChapter) {
      drafts.set(key(projectId, chapter.id), { projectId, chapter: { ...chapter } });
    },
    hasPending() { return drafts.size > 0; },
    flush(): Promise<void> {
      if (running) return running;
      running = Promise.resolve().then(async () => {
        while (drafts.size) {
          const [id, draft] = drafts.entries().next().value!;
          const saved = await write(draft.projectId, draft.chapter);
          if (drafts.get(id) === draft) {
            drafts.delete(id);
            onSaved(draft.projectId, draft.chapter, saved);
          }
        }
      }).finally(() => { running = null; });
      return running;
    },
  };
}
