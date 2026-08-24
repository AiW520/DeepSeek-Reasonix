// Run: tsx src/__tests__/workbench-background-preferences.test.ts

import {
  DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES,
  WORKBENCH_BACKGROUNDS,
  normalizeWorkbenchBackgroundPreferences,
} from "../lib/workbenchBackground";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

console.log("\nworkbench background preferences");

const defaults = normalizeWorkbenchBackgroundPreferences(null);
ok(
  JSON.stringify(defaults) === JSON.stringify(DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES),
  "missing settings use stable defaults",
);

const clamped = normalizeWorkbenchBackgroundPreferences({
  id: "untrusted-file",
  brightness: 999,
  overlay: -20,
  blur: Number.NaN,
});
ok(clamped.id === "architectural", "unknown background identifiers are rejected");
ok(clamped.brightness === 120, "brightness is clamped to the supported range");
ok(clamped.overlay === 0, "overlay is clamped to the supported range");
ok(clamped.blur === 0, "invalid blur values fall back safely");
ok(WORKBENCH_BACKGROUNDS.length === 5, "the A1 library contains four scenes and a no-image option");
ok(
  WORKBENCH_BACKGROUNDS.every((item) => item.imageUrl === null || item.imageUrl.startsWith("/backgrounds/workbench-")),
  "built-in scenes only reference bundled Workbench assets",
);

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
