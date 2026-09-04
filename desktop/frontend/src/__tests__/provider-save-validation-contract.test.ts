// Run: tsx src/__tests__/provider-save-validation-contract.test.ts

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const source = readFileSync(resolve(here, "../components/SettingsPanel.tsx"), "utf8");

let failed = 0;

function ok(condition: boolean, label: string) {
  if (condition) {
    process.stdout.write(`  PASS  ${label}\n`);
    return;
  }
  process.stderr.write(`  FAIL  ${label}\n`);
  failed += 1;
}

console.log("\nprovider save validation contract");

const strictSave = source.match(
  /async function validateAndSaveProviderStrict[\s\S]*?\n}\n\nfunction modelProviderLabel/,
)?.[0] ?? "";

ok(
  strictSave.includes("app.ValidateAndSaveProvider(provider, key ?? \"\")"),
  "custom provider saves pass through the real chat validation gate",
);
ok(
  !strictSave.includes("app.SaveProvider(") && !strictSave.includes("app.SaveProviderWithKey("),
  "failed validation cannot fall back to persisting an unverified provider",
);
ok(
  source.match(/validateAndSaveProviderStrict\(pv, key\)/g)?.length === 2,
  "both add and edit flows use strict provider validation",
);
ok(
  !source.includes("validateAndSaveProviderDraft"),
  "the former draft-save bypass is absent",
);

if (failed > 0) process.exit(1);
