import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Bot,
  CheckCircle2,
  CircleAlert,
  CirclePause,
  ClipboardCheck,
  Eye,
  Pause,
  Play,
  RotateCcw,
  Save,
  Settings2,
  ShieldCheck,
  Sparkles,
  Trash2,
} from "lucide-react";
import { app, onDevelopmentStudioEvent } from "../lib/bridge";
import { useI18n } from "../lib/i18n";
import type {
  DevelopmentAIConfig,
  DevelopmentAIRuntime,
  DevelopmentStudioConfig,
  DevelopmentStudioEvent,
  DevelopmentStudioSnapshot,
} from "../lib/types";
import { Tooltip } from "./Tooltip";
import { developmentStudioCopy } from "./developmentStudioCopy";
import "./DevelopmentStudioPanel.css";

type StudioView = "timeline" | "findings" | "final" | "settings";

const EMPTY_RUNTIME: DevelopmentAIRuntime = { role: "lead", state: "idle", runs: 0, tokens: 0, cost: 0 };

function formatTokens(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return String(value || 0);
}

function formatCost(runtime: DevelopmentAIRuntime): string {
  if (!runtime.cost) return "0.0000";
  return `${runtime.currency ?? ""}${runtime.cost.toFixed(4)}`;
}

function formatClock(at: number): string {
  if (!at) return "--:--";
  return new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" }).format(at);
}

export function DevelopmentStudioPanel({ tabId }: { tabId?: string }) {
  const { locale } = useI18n();
  const copy = developmentStudioCopy(locale);
  const [snapshot, setSnapshot] = useState<DevelopmentStudioSnapshot | null>(null);
  const [draft, setDraft] = useState<DevelopmentStudioConfig | null>(null);
  const [view, setView] = useState<StudioView>("timeline");
  const [busy, setBusy] = useState<"pause" | "clear" | "final" | "save" | "">("");
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    if (!tabId) return;
    const next = await app.DevelopmentStudio(tabId);
    setSnapshot(next);
    setDraft(next.config);
  }, [tabId]);

  useEffect(() => {
    let cancelled = false;
    setSnapshot(null);
    setMessage("");
    if (!tabId) return;
    void app.DevelopmentStudio(tabId).then((next) => {
      if (cancelled) return;
      setSnapshot(next);
      setDraft(next.config);
    }).catch((error) => !cancelled && setMessage(String(error)));
    return () => { cancelled = true; };
  }, [tabId]);

  useEffect(() => onDevelopmentStudioEvent((event) => {
    if (event.tabId !== tabId) return;
    setSnapshot((current) => {
      if (!current || current.events.some((item) => item.id === event.id)) return current;
      const events = [...current.events, event].slice(-240);
      const key = event.role === "realtime" ? "realtime" : event.role === "final" ? "final" : event.role === "lead" ? "lead" : null;
      if (!key) return { ...current, paused: event.status === "paused" ? true : event.status === "running" ? false : current.paused, events };
      const runtime = { ...current[key], state: event.kind === "error" ? "error" : event.kind === "finding" || event.kind === "verdict" ? "done" : event.status || current[key].state };
      return { ...current, [key]: runtime, events };
    });
  }), [tabId]);

  const events = useMemo(() => {
    if (!snapshot) return [];
    if (view === "findings") return snapshot.events.filter((event) => event.role === "realtime" && (event.kind === "finding" || event.kind === "error" || event.severity));
    if (view === "final") return snapshot.events.filter((event) => event.role === "final");
    return snapshot.events.filter((event) => event.role === "lead" || event.role === "system");
  }, [snapshot, view]);

  const run = async (kind: typeof busy, action: () => Promise<void>) => {
    setBusy(kind);
    setMessage("");
    try {
      await action();
      await load();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy("");
    }
  };

  if (!tabId) return <div className="dev-studio dev-studio--empty">{copy.emptyTab}</div>;
  if (!snapshot || !draft) return <div className="dev-studio dev-studio--empty">{message || copy.loading}</div>;

  const runtimes = [
    { key: "lead", label: copy.lead, icon: Bot, runtime: snapshot.lead ?? EMPTY_RUNTIME },
    { key: "realtime", label: copy.realtime, icon: Eye, runtime: snapshot.realtime ?? EMPTY_RUNTIME },
    { key: "final", label: copy.final, icon: ShieldCheck, runtime: snapshot.final ?? EMPTY_RUNTIME },
  ];

  return (
    <section className="dev-studio" aria-label={copy.title}>
      <header className="dev-studio__header">
        <div className="dev-studio__heading">
          <span className="dev-studio__eyebrow"><Sparkles size={13} />{copy.live}</span>
          <h2>{copy.title}</h2>
          <p>{copy.subtitle}</p>
        </div>
        <div className="dev-studio__actions">
          <Tooltip label={snapshot.paused ? copy.resume : copy.pause}>
            <button className="dev-studio__icon-button" type="button" disabled={Boolean(busy)} onClick={() => void run("pause", () => app.PauseDevelopmentStudio(tabId, !snapshot.paused))} aria-label={snapshot.paused ? copy.resume : copy.pause}>
              {snapshot.paused ? <Play size={16} /> : <Pause size={16} />}
            </button>
          </Tooltip>
          <Tooltip label={copy.runFinal}>
            <button className="dev-studio__icon-button" type="button" disabled={Boolean(busy) || snapshot.paused} onClick={() => void run("final", () => app.RunFinalDevelopmentReview(tabId))} aria-label={copy.runFinal}><ClipboardCheck size={16} /></button>
          </Tooltip>
          <Tooltip label={copy.clear}>
            <button className="dev-studio__icon-button" type="button" disabled={Boolean(busy)} onClick={() => void run("clear", () => app.ClearDevelopmentStudio(tabId))} aria-label={copy.clear}><Trash2 size={15} /></button>
          </Tooltip>
        </div>
      </header>

      <div className="dev-studio__roles">
        {runtimes.map(({ key, label, icon: Icon, runtime }) => (
          <div className={`dev-role dev-role--${key}`} key={key}>
            <span className={`dev-role__state dev-role__state--${runtime.state}`} aria-hidden="true" />
            <Icon size={16} />
            <div className="dev-role__identity"><strong>{label}</strong><span>{runtime.model || copy.followModel}</span></div>
            <div className="dev-role__metric"><span>{copy.tokens}</span><strong>{formatTokens(runtime.tokens)}</strong></div>
            <div className="dev-role__metric"><span>{copy.cost}</span><strong>{formatCost(runtime)}</strong></div>
          </div>
        ))}
      </div>

      <div className="dev-studio__control-line">
        <span className={`dev-studio__global-state${snapshot.paused ? " dev-studio__global-state--paused" : ""}`}>
          {snapshot.paused ? <CirclePause size={14} /> : <CheckCircle2 size={14} />}
          {snapshot.paused ? copy.paused : copy.running}
        </span>
        <span>{copy.readOnlyGuard}</span>
        <span>{copy.budget.replace("{tokens}", formatTokens(draft.turnTokenBudget))}</span>
      </div>

      <nav className="dev-studio__views" aria-label={copy.views}>
        {(["timeline", "findings", "final", "settings"] as StudioView[]).map((item) => (
          <button type="button" key={item} className={view === item ? "dev-studio__view dev-studio__view--active" : "dev-studio__view"} onClick={() => setView(item)}>
            {item === "settings" && <Settings2 size={13} />}{item === "timeline" ? copy.timeline : item === "findings" ? copy.findings : item === "final" ? copy.finalView : copy.settings}
          </button>
        ))}
      </nav>

      {message && <div className="dev-studio__notice"><CircleAlert size={15} />{message}</div>}

      {view === "settings" ? (
        <StudioSettings
          config={draft}
          copy={copy}
          models={snapshot.models.map((model) => model.ref)}
          disabled={Boolean(busy)}
          onChange={setDraft}
          onReset={() => setDraft(snapshot.config)}
          onSave={() => void run("save", () => app.SaveDevelopmentStudioConfig(draft))}
        />
      ) : (
        <div className="dev-studio__feed">
          {events.length === 0 ? <div className="dev-studio__empty-feed">{copy.noEvents}</div> : events.slice().reverse().map((event) => <StudioEvent key={event.id} event={event} copy={copy} />)}
        </div>
      )}
    </section>
  );
}

function StudioEvent({ event, copy }: { event: DevelopmentStudioEvent; copy: ReturnType<typeof developmentStudioCopy> }) {
  const severity = event.severity || (event.status === "failed" ? "blocker" : "");
  const roleLabel = event.role === "lead" ? copy.lead : event.role === "realtime" ? copy.realtime : event.role === "final" ? copy.final : copy.system;
  const severityLabel = severity === "blocker" ? copy.blocker : severity === "high" ? copy.high : severity === "suggestion" ? copy.suggestion : severity ? copy.observe : "";
  return (
    <article className={`dev-event${severity ? ` dev-event--${severity}` : ""}`}>
      <div className="dev-event__rail"><span className={`dev-event__dot dev-event__dot--${event.role}`} /><span /></div>
      <div className="dev-event__content">
        <div className="dev-event__meta">
          <span>{roleLabel}</span>
          {severityLabel && <b>{severityLabel}</b>}
          <time>{formatClock(event.at)}</time>
        </div>
        <h3>{event.title}</h3>
        {event.detail && <p>{event.detail}</p>}
        {event.why && <div className="dev-event__why"><strong>{copy.why}</strong>{event.why}</div>}
        {(event.target || event.paths?.length) && <code>{event.target || event.paths?.join(", ")}</code>}
        {(event.tokens || event.cost) && <div className="dev-event__usage">{formatTokens(event.tokens || 0)} Tokens · {event.currency ?? ""}{(event.cost || 0).toFixed(4)}</div>}
      </div>
    </article>
  );
}

function StudioSettings({ config, copy, models, disabled, onChange, onReset, onSave }: {
  config: DevelopmentStudioConfig;
  copy: ReturnType<typeof developmentStudioCopy>;
  models: string[];
  disabled: boolean;
  onChange: (config: DevelopmentStudioConfig) => void;
  onReset: () => void;
  onSave: () => void;
}) {
  const patchAI = (key: "realtime" | "final", patch: Partial<DevelopmentAIConfig>) => onChange({ ...config, [key]: { ...config[key], ...patch } });
  return (
    <div className="dev-settings">
      <div className="dev-settings__row dev-settings__row--top">
        <label><span>{copy.teachingMode}</span><select value={config.teachingMode} onChange={(event) => onChange({ ...config, teachingMode: event.target.value as DevelopmentStudioConfig["teachingMode"] })}><option value="beginner">{copy.beginner}</option><option value="engineer">{copy.engineer}</option><option value="expert">{copy.expert}</option></select></label>
        <label className="dev-settings__switch"><input type="checkbox" checked={config.enabled} onChange={(event) => onChange({ ...config, enabled: event.target.checked })} /><span />{copy.enableStudio}</label>
      </div>
      {(["realtime", "final"] as const).map((key) => (
        <section className="dev-settings__section" key={key}>
          <div className="dev-settings__section-title"><div><strong>{key === "realtime" ? copy.realtime : copy.final}</strong><span>{copy.readOnly}</span></div><label className="dev-settings__switch"><input type="checkbox" checked={config[key].enabled} onChange={(event) => patchAI(key, { enabled: event.target.checked })} /><span />{copy.enabled}</label></div>
          <div className="dev-settings__grid">
            <label><span>{copy.model}</span><select value={config[key].model} onChange={(event) => patchAI(key, { model: event.target.value })}><option value="">{copy.followModel}</option>{models.map((model) => <option key={model} value={model}>{model}</option>)}</select></label>
            <label><span>{copy.effort}</span><select value={config[key].effort} onChange={(event) => patchAI(key, { effort: event.target.value })}>{["auto", "medium", "high", "xhigh", "max"].map((effort) => <option key={effort} value={effort}>{effort}</option>)}</select></label>
            <label><span>{copy.maxSteps}</span><input type="number" min={1} max={20} value={config[key].maxSteps} onChange={(event) => patchAI(key, { maxSteps: Number(event.target.value) })} /></label>
            <label><span>{copy.outputTokens}</span><input type="number" min={256} max={16000} step={256} value={config[key].maxOutputTokens} onChange={(event) => patchAI(key, { maxOutputTokens: Number(event.target.value) })} /></label>
          </div>
        </section>
      ))}
      <section className="dev-settings__section">
        <div className="dev-settings__section-title"><div><strong>{copy.guardrails}</strong><span>{copy.guardrailsHint}</span></div></div>
        <div className="dev-settings__grid">
          <label><span>{copy.interval}</span><input type="number" min={1000} max={60000} step={500} value={config.minReviewIntervalMs} onChange={(event) => onChange({ ...config, minReviewIntervalMs: Number(event.target.value) })} /></label>
          <label><span>{copy.turnBudget}</span><input type="number" min={5000} max={1000000} step={1000} value={config.turnTokenBudget} onChange={(event) => onChange({ ...config, turnTokenBudget: Number(event.target.value) })} /></label>
        </div>
      </section>
      <div className="dev-settings__actions"><button type="button" onClick={onReset} disabled={disabled}><RotateCcw size={14} />{copy.reset}</button><button className="dev-settings__save" type="button" onClick={onSave} disabled={disabled}><Save size={14} />{copy.save}</button></div>
    </div>
  );
}
