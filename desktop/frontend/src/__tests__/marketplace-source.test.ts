// Run: tsx src/__tests__/marketplace-source.test.ts

import { marketplaceInstallSource } from "../lib/bridge";

const cases: Array<[string, string, string, string]> = [
  ["https://github.com/runesleo/claude-video-kit", "skill", "abc123", "https://github.com/runesleo/claude-video-kit/tree/abc123"],
  ["https://github.com/runesleo/claude-video-kit/tree/old-revision/skills/demo", "skill", "abc123", "https://github.com/runesleo/claude-video-kit/tree/abc123"],
  [" https://github.com/obra/superpowers/// ", "plugin", "abc123", "https://github.com/obra/superpowers"],
];

let failed = 0;
for (const [repository, kind, commit, expected] of cases) {
  const actual = marketplaceInstallSource(repository, kind, commit);
  if (actual !== expected) {
    console.error(`FAIL ${repository}: expected ${expected}, got ${actual}`);
    failed += 1;
  } else {
    console.log(`PASS ${kind} source normalization`);
  }
}
if (failed) process.exit(1);
