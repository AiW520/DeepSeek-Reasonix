export const WORKBENCH_BACKGROUND_IDS = [
  "architectural",
  "midnight-grid",
  "monochrome-studio",
  "night-city",
  "none",
] as const;

export type WorkbenchBackgroundId = (typeof WORKBENCH_BACKGROUND_IDS)[number];

export type WorkbenchBackgroundPreferences = {
  id: WorkbenchBackgroundId;
  brightness: number;
  overlay: number;
  blur: number;
};

export type WorkbenchBackgroundDefinition = {
  id: WorkbenchBackgroundId;
  imageUrl: string | null;
  nameKey:
    | "settings.workbenchBackground.architectural"
    | "settings.workbenchBackground.midnightGrid"
    | "settings.workbenchBackground.monochromeStudio"
    | "settings.workbenchBackground.nightCity"
    | "settings.workbenchBackground.none";
};

export const WORKBENCH_BACKGROUNDS: readonly WorkbenchBackgroundDefinition[] = [
  {
    id: "architectural",
    imageUrl: "/backgrounds/workbench-architectural.webp",
    nameKey: "settings.workbenchBackground.architectural",
  },
  {
    id: "midnight-grid",
    imageUrl: "/backgrounds/workbench-midnight-grid.webp",
    nameKey: "settings.workbenchBackground.midnightGrid",
  },
  {
    id: "monochrome-studio",
    imageUrl: "/backgrounds/workbench-monochrome-studio.webp",
    nameKey: "settings.workbenchBackground.monochromeStudio",
  },
  {
    id: "night-city",
    imageUrl: "/backgrounds/workbench-night-city.webp",
    nameKey: "settings.workbenchBackground.nightCity",
  },
  {
    id: "none",
    imageUrl: null,
    nameKey: "settings.workbenchBackground.none",
  },
] as const;

export const DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES: WorkbenchBackgroundPreferences = {
  id: "architectural",
  brightness: 100,
  overlay: 18,
  blur: 0,
};

export const WORKBENCH_BACKGROUND_STORAGE_KEY = "reasonix.workbenchBackground.v1";
export const WORKBENCH_BACKGROUND_EVENT = "reasonix:workbench-background";

function clamp(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  return Math.round(Math.min(max, Math.max(min, value)));
}

export function isWorkbenchBackgroundId(value: unknown): value is WorkbenchBackgroundId {
  return typeof value === "string" && (WORKBENCH_BACKGROUND_IDS as readonly string[]).includes(value);
}

export function normalizeWorkbenchBackgroundPreferences(value: unknown): WorkbenchBackgroundPreferences {
  const source = value && typeof value === "object" ? (value as Partial<WorkbenchBackgroundPreferences>) : {};
  return {
    id: isWorkbenchBackgroundId(source.id) ? source.id : DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES.id,
    brightness: clamp(source.brightness, 60, 120, DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES.brightness),
    overlay: clamp(source.overlay, 0, 70, DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES.overlay),
    blur: clamp(source.blur, 0, 12, DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES.blur),
  };
}

export function getWorkbenchBackgroundPreferences(): WorkbenchBackgroundPreferences {
  if (typeof localStorage === "undefined") return { ...DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES };
  try {
    const stored = localStorage.getItem(WORKBENCH_BACKGROUND_STORAGE_KEY);
    return stored
      ? normalizeWorkbenchBackgroundPreferences(JSON.parse(stored))
      : { ...DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES };
  } catch {
    return { ...DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES };
  }
}

export function applyWorkbenchBackgroundPreferences(value: unknown): WorkbenchBackgroundPreferences {
  const preferences = normalizeWorkbenchBackgroundPreferences(value);
  if (typeof document === "undefined") return preferences;

  const definition = WORKBENCH_BACKGROUNDS.find((item) => item.id === preferences.id) ?? WORKBENCH_BACKGROUNDS[0];
  const root = document.documentElement;
  root.setAttribute("data-workbench-background", preferences.id);
  root.style.setProperty(
    "--workbench-bg-image",
    definition.imageUrl ? `url("${definition.imageUrl}")` : "none",
  );
  root.style.setProperty("--workbench-bg-brightness", String(preferences.brightness / 100));
  root.style.setProperty("--workbench-bg-overlay", String(preferences.overlay / 100));
  root.style.setProperty("--workbench-bg-blur", `${preferences.blur}px`);
  return preferences;
}

export function setWorkbenchBackgroundPreferences(value: unknown): WorkbenchBackgroundPreferences {
  const preferences = applyWorkbenchBackgroundPreferences(value);
  try {
    localStorage.setItem(WORKBENCH_BACKGROUND_STORAGE_KEY, JSON.stringify(preferences));
  } catch {
    // Settings still apply for the current session when storage is unavailable.
  }
  if (typeof window !== "undefined") {
    window.dispatchEvent(new CustomEvent(WORKBENCH_BACKGROUND_EVENT, { detail: preferences }));
  }
  return preferences;
}

export function resetWorkbenchBackgroundPreferences(): WorkbenchBackgroundPreferences {
  return setWorkbenchBackgroundPreferences(DEFAULT_WORKBENCH_BACKGROUND_PREFERENCES);
}

export function initWorkbenchBackgroundPreferences(): void {
  applyWorkbenchBackgroundPreferences(getWorkbenchBackgroundPreferences());
}
