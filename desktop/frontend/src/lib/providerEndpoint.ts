export interface ProviderEndpointConfig {
  kind: string;
  baseUrl: string;
  requestUrl?: string;
  chatUrl?: string;
}

export type ProviderAddressMode = "base" | "endpoint";

export function trimmedBaseURL(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

export function providerRequestURLFromConfig(
  kind: string,
  baseUrl: string,
  requestUrl: string,
  legacyChatUrl = "",
): string {
  const exactRequestURL = requestUrl.trim();
  if (exactRequestURL) return exactRequestURL;
  if (kind.trim().toLowerCase() === "openai") {
    const legacyOpenAIRequestURL = legacyChatUrl.trim().replace(/\/+$/, "");
    if (legacyOpenAIRequestURL) return legacyOpenAIRequestURL;
  }
  const base = trimmedBaseURL(baseUrl);
  if (!base) return "";
  switch (kind.trim().toLowerCase()) {
    case "anthropic":
      return base.endsWith("/v1") ? `${base}/messages` : `${base}/v1/messages`;
    case "responses":
    case "dashscope-responses":
      return `${base}/responses`;
    default:
      return `${base}/chat/completions`;
  }
}

export function providerAddressModeFromConfig(config: ProviderEndpointConfig | undefined): ProviderAddressMode {
  if (!config) return "base";
  if ((config.requestUrl ?? "").trim()) return "endpoint";
  if (config.kind.trim().toLowerCase() === "openai" && (config.chatUrl ?? "").trim()) return "endpoint";
  return "base";
}

export function providerAddressInputFromConfig(config: ProviderEndpointConfig | undefined): string {
  if (!config) return "";
  return providerAddressModeFromConfig(config) === "endpoint"
    ? providerRequestURLFromConfig(config.kind, config.baseUrl, config.requestUrl ?? "", config.chatUrl ?? "")
    : config.baseUrl;
}

export function providerRequestURLFromInput(kind: string, address: string, mode: ProviderAddressMode): string {
  const value = address.trim();
  if (!value) return "";
  return mode === "base" ? providerRequestURLFromConfig(kind, value, "") : value;
}

export function providerBaseURLFromInput(
  initial: ProviderEndpointConfig | undefined,
  kind: string,
  address: string,
  mode: ProviderAddressMode,
): string {
  const value = address.trim();
  if (!value) return "";
  if (mode === "base") return trimmedBaseURL(value);
  return providerBaseURLForSave(initial, kind, value);
}

export function providerPrimaryModelsURL(baseUrl: string, modelsUrl = ""): string {
  const exact = modelsUrl.trim();
  if (exact) return exact;
  const base = trimmedBaseURL(baseUrl);
  return base ? `${base}/models` : "";
}

export function providerBaseURLFromRequestURL(kind: string, requestUrl: string): string {
  const exactRequestURL = requestUrl.trim();
  if (!exactRequestURL) return "";
  const normalizedKind = kind.trim().toLowerCase();
  const suffixes = normalizedKind === "anthropic"
    ? ["/v1/messages"]
    : normalizedKind === "responses" || normalizedKind === "dashscope-responses"
      ? ["/responses"]
      : ["/chat/completions"];
  try {
    const parsed = new URL(exactRequestURL);
    const pathname = parsed.pathname.replace(/\/+$/, "");
    const suffix = suffixes.find((candidate) => pathname.endsWith(candidate));
    parsed.pathname = suffix ? pathname.slice(0, -suffix.length) || "/" : pathname || "/";
    parsed.search = "";
    parsed.hash = "";
    return trimmedBaseURL(parsed.toString());
  } catch {
    const suffix = suffixes.find((candidate) => exactRequestURL.endsWith(candidate));
    if (suffix) return trimmedBaseURL(exactRequestURL.slice(0, -suffix.length));
  }
  return trimmedBaseURL(exactRequestURL);
}

export function providerBaseURLForSave(
  initial: ProviderEndpointConfig | undefined,
  effectiveKind: string,
  effectiveRequestUrl: string,
): string {
  const requestUrl = effectiveRequestUrl.trim();
  if (initial) {
    const initialRequestUrl = providerRequestURLFromConfig(
      initial.kind,
      initial.baseUrl,
      initial.requestUrl ?? "",
      initial.chatUrl ?? "",
    );
    const kindUnchanged = initial.kind.trim().toLowerCase() === effectiveKind.trim().toLowerCase();
    if (kindUnchanged && initialRequestUrl === requestUrl) {
      return initial.baseUrl.trim();
    }
  }
  return providerBaseURLFromRequestURL(effectiveKind, requestUrl);
}
