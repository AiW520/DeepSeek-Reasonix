import { lazy, Suspense } from "react";
import logoIcon from "../assets/reasonix-icon.png";
import { useT } from "../lib/i18n";
import type { WorkbenchModule } from "../lib/workbench";

const WorkbenchHome = lazy(() => import("./WorkbenchHome").then((module) => ({ default: module.WorkbenchHome })));

// Welcome is the empty-state landing: a one-liner, the input affordances
// (/ commands, @ files, Enter), and a few clickable example prompts that send
// immediately so a first turn is one click away.

export function Welcome({ onPrompt, onOpenModule, onOpenModelSettings, variant = "default" }: { onPrompt: (text: string) => void; onOpenModule?: (module: WorkbenchModule) => void; onOpenModelSettings?: () => void; variant?: "default" | "creation" }) {
  const t = useT();
  if (variant === "creation") {
    // Headline lives above the hero Composer in App footer (same stack).
    void onPrompt;
    void t;
    return null;
  }

  const examples = [t("welcome.ex1"), t("welcome.ex2"), t("welcome.ex3"), t("welcome.ex4")];
  return (
    <div className={`welcome welcome--brand${onOpenModule ? " welcome--workbench-home" : ""}`}>
      <span className="welcome__brand">
        <img src={logoIcon} className="welcome__brand-logo" alt="Reasonix" draggable={false} />
        <strong className="welcome__brand-name">Reasonix</strong>
      </span>
      <h2 className="welcome__title">{t("welcome.title")}</h2>
      <div className="welcome__tag">{t("welcome.tagline")}</div>

      {!onOpenModule && <div className="welcome__hints">
        <span>
          <kbd>/</kbd> {t("welcome.hintCommands")}
        </span>
        <span>
          <kbd>@</kbd> {t("welcome.hintFiles")}
        </span>
        <span>
          <kbd>⏎</kbd> {t("welcome.hintSend")}
        </span>
      </div>}

      {!onOpenModule && <div className="welcome__examples">
        {examples.map((ex) => (
          <button key={ex} className="welcome__ex" onClick={() => onPrompt(ex)}>
            {ex}
          </button>
        ))}
      </div>}
      {onOpenModule && <Suspense fallback={null}><WorkbenchHome onOpenModule={onOpenModule} onOpenModelSettings={onOpenModelSettings} /></Suspense>}
    </div>
  );
}
