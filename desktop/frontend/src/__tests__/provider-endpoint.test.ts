import {
  providerAddressInputFromConfig,
  providerAddressModeFromConfig,
  providerBaseURLForSave,
  providerBaseURLFromInput,
  providerBaseURLFromRequestURL,
  providerPrimaryModelsURL,
  providerRequestURLFromConfig,
  providerRequestURLFromInput,
} from "../lib/providerEndpoint";
let failed = 0;
function eq(actual: unknown, expected: unknown, label: string) {
  if (actual === expected) {
    process.stdout.write(`  PASS  ${label}\n`);
    return;
  }
  process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}\n`);
  failed += 1;
}
console.log("\nprovider endpoint");

eq(providerRequestURLFromConfig("openai", "https://proxy.example.com/v1", ""), "https://proxy.example.com/v1/chat/completions", "legacy OpenAI base URLs expose their effective request URL");
eq(providerRequestURLFromConfig("anthropic", "https://proxy.example.com/v1", ""), "https://proxy.example.com/v1/messages", "legacy Anthropic base URLs expose their effective request URL");
eq(providerRequestURLFromConfig("responses", "https://proxy.example.com/v1", ""), "https://proxy.example.com/v1/responses", "legacy Responses base URLs expose their effective request URL");
eq(providerRequestURLFromConfig("openai", "https://proxy.example.com/v1", "", "https://legacy.example.com/chat/completions/"), "https://legacy.example.com/chat/completions", "legacy OpenAI chat URLs preserve historical trailing-slash normalization");
eq(providerRequestURLFromConfig("anthropic", "https://proxy.example.com/v1", "", "https://stale.example.com/chat/completions"), "https://proxy.example.com/v1/messages", "legacy Anthropic chat URLs remain ignored");
eq(providerRequestURLFromConfig("openai", "", "https://proxy.example.com/custom/chat/?token=1", "https://legacy.example.com/chat/completions"), "https://proxy.example.com/custom/chat/?token=1", "explicit request URLs remain unchanged and take priority");
eq(providerBaseURLFromRequestURL("openai", "https://proxy.example.com/v1/chat/completions?token=1"), "https://proxy.example.com/v1", "request URLs derive a query-free base URL for model discovery");

eq(providerBaseURLForSave({ kind: "openai", baseUrl: "https://api.deepseek.com/v1", chatUrl: "https://gateway.example/custom/chat/completions" }, "openai", "https://gateway.example/custom/chat/completions"), "https://api.deepseek.com/v1", "unchanged legacy providers preserve their independent base URL");
eq(providerBaseURLForSave({ kind: "anthropic", baseUrl: "https://models.example/v1/", requestUrl: "https://gateway.example/custom/messages?token=1" }, "anthropic", "https://gateway.example/custom/messages?token=1"), "https://models.example/v1/", "unchanged explicit request URLs preserve their independent base URL exactly");
eq(providerBaseURLForSave({ kind: "openai", baseUrl: "https://models.example/v1", requestUrl: "https://gateway.example/old/chat/completions" }, "openai", "https://gateway.example/new/chat/completions"), "https://gateway.example/new", "changing the request URL derives a new base URL");
eq(providerBaseURLForSave({ kind: "openai", baseUrl: "https://models.example/v1", requestUrl: "https://gateway.example/v1/chat/completions" }, "anthropic", "https://gateway.example/v1/chat/completions"), "https://gateway.example/v1/chat/completions", "changing protocol derives a new base URL under the new protocol");
eq(providerBaseURLForSave(undefined, "responses", "https://gateway.example/v1/responses"), "https://gateway.example/v1", "new providers derive their base URL from the exact request URL");
eq(providerAddressModeFromConfig(undefined), "base", "new providers default to the simpler base URL mode");
eq(providerAddressInputFromConfig({ kind: "openai", baseUrl: "https://proxy.example.com/v1" }), "https://proxy.example.com/v1", "base-only providers keep their base URL in the editor");
eq(providerAddressModeFromConfig({ kind: "openai", baseUrl: "https://proxy.example.com/v1", requestUrl: "https://proxy.example.com/custom/chat" }), "endpoint", "explicit request URLs select endpoint mode");
eq(providerRequestURLFromInput("openai", "https://api.tu-zi.com/v1", "base"), "https://api.tu-zi.com/v1/chat/completions", "OpenAI base mode appends the chat completions path");
eq(providerRequestURLFromInput("responses", "https://api.tu-zi.com/v1", "base"), "https://api.tu-zi.com/v1/responses", "Responses base mode appends the responses path");
eq(providerRequestURLFromInput("anthropic", "https://proxy.example.com", "base"), "https://proxy.example.com/v1/messages", "Anthropic base mode appends the messages path");
eq(providerRequestURLFromInput("openai", "https://proxy.example.com/special", "endpoint"), "https://proxy.example.com/special", "endpoint mode preserves custom paths");
eq(providerBaseURLFromInput(undefined, "openai", "https://api.tu-zi.com/v1/", "base"), "https://api.tu-zi.com/v1", "base mode trims only trailing slashes");
eq(providerPrimaryModelsURL("https://api.tu-zi.com/v1", ""), "https://api.tu-zi.com/v1/models", "model discovery preview follows the normalized base URL");
eq(providerPrimaryModelsURL("https://api.tu-zi.com/v1", "https://models.example/catalog"), "https://models.example/catalog", "explicit model discovery endpoints take priority");

if (failed > 0) process.exit(1);
