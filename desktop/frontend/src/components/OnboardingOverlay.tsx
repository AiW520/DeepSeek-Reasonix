import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import logo from "../assets/reasonix-icon.png";
import { useT } from "../lib/i18n";
import { app, openExternal } from "../lib/bridge";

// Full-window first-run guide: DeepSeek stays the fastest path, while users can
// open the provider settings or defer setup without being trapped in the gate.
export function OnboardingOverlay({
  onComplete,
  onChooseProvider,
  onSkip,
}: {
  onComplete: () => void;
  onChooseProvider: () => void;
  onSkip: () => void;
}) {
  const t = useT();
  const [value, setValue] = useState("");
  const [state, setState] = useState<"idle" | "validating" | "error">("idle");
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const submittingRef = useRef(false);

  useLayoutEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    inputRef.current?.focus();
    return () => { if (previous?.isConnected) previous.focus(); };
  }, []);

  useEffect(() => {
    if (state !== "error") return;
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [state]);

  useEffect(() => {
    const focusable = () => Array.from(dialogRef.current?.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), [tabindex="0"]',
    ) ?? []);
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        if (!submittingRef.current) onSkip();
        return;
      }
      if (event.key !== "Tab") return;
      const items = focusable();
      const first = items[0];
      const last = items[items.length - 1];
      if (!first || !last) {
        event.preventDefault();
        dialogRef.current?.focus();
      } else if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    const retainFocus = (event: FocusEvent) => {
      if (event.target instanceof Node && !dialogRef.current?.contains(event.target)) {
        (focusable()[0] ?? dialogRef.current)?.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown, true);
    document.addEventListener("focusin", retainFocus);
    return () => {
      document.removeEventListener("keydown", onKeyDown, true);
      document.removeEventListener("focusin", retainFocus);
    };
  }, [onSkip]);

  const submit = useCallback(async () => {
    if (submittingRef.current) return;
    const key = value.trim();
    if (!key) {
      setError(t("onboarding.error.empty"));
      setState("error");
      inputRef.current?.focus();
      return;
    }
    submittingRef.current = true;
    setState("validating");
    setError(null);
    try {
      await app.ConnectKey(key);
      onComplete();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (/status\s*401|status\s*403|invalid/i.test(msg)) {
        setError(t("onboarding.error.invalid"));
      } else if (/network|unreachable|timeout|dial/i.test(msg)) {
        setError(t("onboarding.error.network"));
      } else {
        setError(msg || t("onboarding.error.unknown"));
      }
      setState("error");
    } finally {
      submittingRef.current = false;
    }
  }, [t, value, onComplete]);

  return (
    <div ref={dialogRef} className="onboarding" role="dialog" aria-modal="true" aria-labelledby="onboarding-title" aria-describedby="onboarding-description" aria-busy={state === "validating"} tabIndex={-1}>
      <div className="onboarding__card">
        <img src={logo} className="onboarding__logo" alt="Reasonix" draggable={false} />
        <div id="onboarding-title" className="onboarding__title">{t("onboarding.title")}</div>
        <div id="onboarding-description" className="onboarding__tag">{t("onboarding.tagline")}</div>

        <label className="onboarding__label" htmlFor="onboarding-key">
          {t("onboarding.inputLabel")}
        </label>
        <input
          id="onboarding-key"
          ref={inputRef}
          className="onboarding__input"
          type="password"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={state === "error"}
          aria-describedby={state === "error" ? "onboarding-error" : undefined}
          placeholder={t("onboarding.inputPlaceholder")}
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            if (state === "error") setState("idle");
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && state !== "validating") {
              e.preventDefault();
              void submit();
            }
          }}
          disabled={state === "validating"}
        />

        {state === "error" && error && (
          <div id="onboarding-error" className="onboarding__error" role="alert">
            {error}
          </div>
        )}

        <button
          className="onboarding__submit"
          onClick={() => void submit()}
          disabled={state === "validating"}
        >
          {state === "validating" ? (
            <>
              <span className="onboarding__spinner" />
              {t("onboarding.validating")}
            </>
          ) : (
            t("onboarding.submit")
          )}
        </button>

        <button
          type="button"
          className="onboarding__provider"
          onClick={onChooseProvider}
          disabled={state === "validating"}
        >
          {t("onboarding.chooseProvider")}
        </button>

        <div className="onboarding__links">
          <button
            type="button"
            className="onboarding__link"
            onClick={() => openExternal("https://platform.deepseek.com/api_keys")}
          >
            {t("onboarding.getKey")}
          </button>
          <span className="onboarding__sep">·</span>
          <span className="onboarding__privacy">{t("onboarding.privacy")}</span>
        </div>

        <button
          type="button"
          className="onboarding__skip"
          onClick={onSkip}
          disabled={state === "validating"}
        >
          {t("onboarding.skip")}
        </button>
      </div>
    </div>
  );
}
