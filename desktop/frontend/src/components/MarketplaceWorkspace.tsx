import { useCallback, useEffect, useState } from "react";
import { Blocks, RefreshCw, ShieldCheck, Sparkles, X } from "lucide-react";
import { app } from "../lib/bridge";
import type { WorkbenchModule } from "../lib/workbench";
import { MarketplaceSection } from "./MarketplaceSection";
import "./MarketplaceWorkspace.css";
import "./SuperWorkspace.css";

export function MarketplaceWorkspace({ mode, onModeChange, onClose }: { mode: "plugins" | "skills"; onModeChange: (mode: WorkbenchModule) => void; onClose: () => void }) {
  const [installedPlugins, setInstalledPlugins] = useState<string[]>([]);
  const [installedSkills, setInstalledSkills] = useState<string[]>([]);
  const [refreshing, setRefreshing] = useState(false);
  const reload = useCallback(async () => {
    setRefreshing(true);
    try {
      const [plugins, skills] = await Promise.all([app.Plugins(), app.SkillsSettings()]);
      setInstalledPlugins((plugins || []).map((item) => item.name));
      setInstalledSkills((skills.skills || []).map((item) => item.name));
    } finally { setRefreshing(false); }
  }, []);
  useEffect(() => { void reload(); }, [reload]);
  return (
    <section className="marketplace-workspace" aria-label={mode === "plugins" ? "插件广场" : "Skill 广场"}>
      <header className="marketplace-workspace__header">
        <div className="marketplace-workspace__identity"><span><ShieldCheck size={19} /></span><div><small>REASONIX MARKETPLACE</small><h1>能力广场</h1></div></div>
        <nav className="marketplace-workspace__tabs" aria-label="能力类型">
          <button type="button" className={mode === "plugins" ? "is-active" : ""} onClick={() => onModeChange("plugins")}><Blocks size={16} />插件广场</button>
          <button type="button" className={mode === "skills" ? "is-active" : ""} onClick={() => onModeChange("skills")}><Sparkles size={16} />Skill 广场</button>
        </nav>
        <div className="marketplace-workspace__actions"><button type="button" title="刷新安装状态" aria-label="刷新安装状态" disabled={refreshing} onClick={() => void reload()}><RefreshCw className={refreshing ? "spin" : ""} size={16} /></button><button type="button" aria-label="关闭能力广场" onClick={onClose}><X size={18} /></button></div>
      </header>
      <main className="marketplace-workspace__body"><MarketplaceSection kind={mode === "plugins" ? "plugin" : "skill"} installedNames={mode === "plugins" ? installedPlugins : installedSkills} onInstalled={reload} /></main>
    </section>
  );
}
