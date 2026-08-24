import { useEffect, useMemo, useState } from "react";
import { Check, Code2, Download, Film, Search, ShieldCheck, Sparkles, X } from "lucide-react";
import { app } from "../lib/bridge";
import type { MarketplaceEntry } from "../lib/types";

type PlanAction = { kind?: string; action?: string; name?: string; riskLevel?: string; riskReasons?: string[]; skillCount?: number; agentCount?: number; commandCount?: number; hookCount?: number; toolCount?: number; runtime?: { command?: string; fullTrust?: boolean }; commit?: string };
type Plan = { ok?: boolean; status?: string; planId?: string; actions: PlanAction[]; warnings?: string[]; error?: string };

const categories = ["all", "work", "programming", "video"] as const;

function parsePlan(raw: string): Plan {
  try { const p = JSON.parse(raw) as Plan; return { ...p, actions: Array.isArray(p.actions) ? p.actions : [] }; }
  catch { return { actions: [], error: "无法读取安全预检结果" }; }
}

export function MarketplaceSection({ kind, installedNames = [], onInstalled }: { kind: "plugin" | "skill"; installedNames?: string[]; onInstalled: () => Promise<void> | void }) {
  const [entries, setEntries] = useState<MarketplaceEntry[]>([]);
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState<(typeof categories)[number]>("all");
  const [selected, setSelected] = useState<MarketplaceEntry | null>(null);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState<string[]>([]);

  useEffect(() => { void app.MarketplaceCatalog(kind, "").then((v) => setEntries(v.entries)).catch((e) => setError(String(e?.message ?? e))); }, [kind]);
  const filtered = useMemo(() => entries.filter((e) => (category === "all" || e.category === category) && (!query.trim() || `${e.name} ${e.description} ${e.author}`.toLowerCase().includes(query.trim().toLowerCase()))), [entries, category, query]);
  const installed = (entry: MarketplaceEntry) => installedNames.some((n) => n.toLowerCase() === entry.name.toLowerCase()) || done.includes(entry.id);

  const preflight = async (entry: MarketplaceEntry) => {
    setBusy(true); setError("");
    try { setSelected(entry); setPlan(parsePlan(await app.PlanMarketplaceInstall(entry.id))); }
    catch (e) { setSelected(null); setError(String((e as Error)?.message ?? e)); }
    finally { setBusy(false); }
  };
  const install = async () => {
    if (!selected || !plan?.planId) return;
    setBusy(true); setError("");
    try { await app.InstallMarketplace(selected.id, plan.planId); setDone((v) => [...v, selected.id]); setSelected(null); setPlan(null); await onInstalled(); }
    catch (e) { setError(String((e as Error)?.message ?? e)); }
    finally { setBusy(false); }
  };

  return <section className="marketplace">
    <div className="marketplace__hero">
      <div><span className="marketplace__eyebrow"><Sparkles size={14} /> CURATED MARKETPLACE</span><h3>{kind === "plugin" ? "插件广场" : "Skill 广场"}</h3><p>精选开源能力，固定 GitHub 快照。安装前先检查权限、运行时与外部依赖。</p></div>
      <div className="marketplace__trust"><ShieldCheck size={20} /><span><strong>安全预检</strong><small>白名单来源 · 两次确认</small></span></div>
    </div>
    {error && <div className="banner banner--error">{error}</div>}
    <div className="marketplace__toolbar">
      <label className="marketplace__search"><Search size={16}/><input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="搜索资源、作者或用途" /></label>
      <div className="marketplace__categories">{categories.map((c) => <button type="button" key={c} className={category === c ? "is-active" : ""} onClick={() => setCategory(c)}>{c === "all" ? "全部" : c === "work" ? "工作" : c === "programming" ? "编程" : "视频"}</button>)}</div>
    </div>
    <div className="marketplace__grid">{filtered.map((entry) => <article className="marketplace-card" key={entry.id}>
      <div className="marketplace-card__top"><span className={`marketplace-card__icon marketplace-card__icon--${entry.category}`}>{entry.category === "video" ? <Film/> : entry.category === "programming" ? <Code2/> : <Sparkles/>}</span><span className={`marketplace-risk marketplace-risk--${entry.risk}`}>{entry.risk === "low" ? "低风险" : entry.risk === "medium" ? "中风险" : "高风险"}</span></div>
      <h4>{entry.name}</h4><p>{entry.description}</p>
      <div className="marketplace-card__chips">{entry.capabilities?.map((x) => <span key={x}>{x}</span>)}</div>
      <div className="marketplace-card__meta"><span>{entry.author}</span><span>{entry.license}</span><code>{entry.commit.slice(0, 8)}</code></div>
      <button className="marketplace-card__install" type="button" disabled={busy || installed(entry)} onClick={() => void preflight(entry)}>{installed(entry) ? <><Check size={16}/> 已安装</> : <><ShieldCheck size={16}/> 安全预检</>}</button>
    </article>)}</div>
    {filtered.length === 0 && <div className="mem-empty">没有匹配的精选资源</div>}
    {selected && <div className="marketplace-dialog-backdrop" onMouseDown={(e) => { if (e.target === e.currentTarget && !busy) { setSelected(null); setPlan(null); } }}>
      <div className="marketplace-dialog" role="dialog" aria-modal="true" aria-label="安装安全确认">
        <header><div><span className="marketplace__eyebrow"><ShieldCheck size={14}/> SECURITY REVIEW</span><h3>安装前安全确认</h3></div><button type="button" aria-label="关闭" disabled={busy} onClick={() => { setSelected(null); setPlan(null); }}><X size={18}/></button></header>
        {!plan ? <div className="marketplace-dialog__loading">正在分析仓库与能力清单...</div> : <>
          <div className="marketplace-dialog__source"><strong>{selected.name}</strong><span>{selected.author} · {selected.license}</span><code>{selected.commit}</code></div>
          <div className="marketplace-dialog__actions">{plan.actions.map((a, i) => <div className="marketplace-plan" key={`${a.name}-${i}`}><div><strong>{a.name || a.action || a.kind}</strong><span className={`marketplace-risk marketplace-risk--${a.riskLevel || selected.risk}`}>{a.riskLevel || selected.risk}</span></div><p>{[a.skillCount && `${a.skillCount} Skills`, a.agentCount && `${a.agentCount} Agents`, a.commandCount && `${a.commandCount} Commands`, a.hookCount && `${a.hookCount} Hooks`, a.toolCount && `${a.toolCount} MCP`].filter(Boolean).join(" · ") || "安装一个受审查的能力"}</p>{a.runtime?.fullTrust && <div className="marketplace-fulltrust">FULL TRUST · {a.runtime.command}</div>}{a.riskReasons?.map((r) => <small key={r}>• {r}</small>)}</div>)}</div>
          {plan.warnings?.map((w) => <div className="banner banner--warning" key={w}>{w}</div>)}
          <div className="marketplace-dialog__notice"><ShieldCheck size={18}/><span>确认后仅安装本次预检的同一份快照。计划或 commit 变化会自动拒绝。</span></div>
          <footer><button className="btn" type="button" disabled={busy} onClick={() => { setSelected(null); setPlan(null); }}>取消</button><button className="btn btn--primary" type="button" disabled={busy || !plan.ok || !plan.planId} onClick={() => void install()}><Download size={16}/>{busy ? "安装中..." : "确认并安装"}</button></footer>
        </>}
      </div>
    </div>}
  </section>;
}
