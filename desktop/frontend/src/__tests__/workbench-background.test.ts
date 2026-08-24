// Run: tsx src/__tests__/workbench-background.test.ts

import { existsSync, readFileSync, statSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const testDir = dirname(fileURLToPath(import.meta.url));
const frontendDir = resolve(testDir, "../..");
const componentSource = readFileSync(resolve(testDir, "../components/ThemeBackground.tsx"), "utf8");
const appearanceSource = readFileSync(resolve(testDir, "../components/AppearanceOverview.tsx"), "utf8");
const preferencesSource = readFileSync(resolve(testDir, "../lib/workbenchBackground.ts"), "utf8");
const stylesSource = readFileSync(resolve(testDir, "../styles.css"), "utf8");
const workbenchStyles = stylesSource.slice(stylesSource.lastIndexOf("Super Workbench scene"));
const backgroundPath = resolve(frontendDir, "public/backgrounds/workbench-architectural.webp");

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

console.log("\nworkbench background contract");

ok(componentSource.includes('className="theme-bg__workbench"'), "theme background mounts the Workbench layer");
ok(existsSync(backgroundPath), "bundled Workbench background exists");
ok(existsSync(backgroundPath) && statSync(backgroundPath).size < 350_000, "background stays within the desktop asset budget");
ok(stylesSource.includes('url("/backgrounds/workbench-architectural.webp")'), "stylesheet references the bundled image");
ok(stylesSource.includes(".app--workbench .theme-bg__workbench"), "image is scoped to Workbench mode");
ok(
  stylesSource.includes(':root[data-theme-pack][data-theme-has-bg="true"] .app--workbench .theme-bg__workbench'),
  "an explicitly selected theme pack can override the default Workbench scene",
);
ok(
  workbenchStyles.includes(':root:not([data-theme-has-bg="true"])[data-platform="windows"] .app.app--workbench') &&
    workbenchStyles.includes("--windows-workbench-pane-alpha"),
  "Windows redraws the default Workbench scene without overriding theme packs",
);
ok(!workbenchStyles.includes("backdrop-filter"), "Workbench does not add unsupported backdrop blur");
ok(
  appearanceSource.includes('className="workbench-background__grid"') &&
    appearanceSource.includes('type="range"'),
  "Appearance settings expose the background library and tuning controls",
);
ok(
  preferencesSource.includes("WORKBENCH_BACKGROUND_STORAGE_KEY") &&
    preferencesSource.includes("normalizeWorkbenchBackgroundPreferences"),
  "Workbench preferences are persisted through a normalized settings boundary",
);
ok(
  stylesSource.includes(':root:not([data-theme-has-bg="true"]) .app--workbench .theme-bg::after'),
  "color-only theme packs keep Workbench background controls active",
);

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
