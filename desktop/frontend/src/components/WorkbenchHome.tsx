import { Blocks, BrainCircuit, Image, Presentation, Settings2, Sparkles, WandSparkles } from "lucide-react";
import type { WorkbenchModule } from "../lib/workbench";
import "./WorkbenchHome.css";
import "./WorkbenchHomeOverrides.css";

const MODULES: Array<{ id: WorkbenchModule; title: string; eyebrow: string; description: string; meta: string; icon: typeof BrainCircuit; tone: string }> = [
  { id: "project-analysis", title: "项目深度分析", eyebrow: "PROJECT ATLAS", description: "扫描代码结构、技术栈与关键证据，建立可学习的项目地图。", meta: "本地只读分析", icon: BrainCircuit, tone: "teal" },
  { id: "image", title: "AI 图片创作", eyebrow: "GPT-IMAGE-2", description: "生成产品图、海报、插画和透明背景素材，并保存到本地作品库。", meta: "Tuzi Coding", icon: Image, tone: "coral" },
  { id: "ppt", title: "PPT 智能制作", eyebrow: "PRESENTATION", description: "从主题生成演示结构，逐页编辑内容并导出标准 PPTX 文件。", meta: "可编辑大纲", icon: Presentation, tone: "gold" },
  { id: "plugins", title: "插件广场", eyebrow: "EXTENSIONS", description: "安装工作、编程和视频插件，先做权限与运行时安全预检。", meta: "确认后安装", icon: Blocks, tone: "blue" },
  { id: "skills", title: "Skill 广场", eyebrow: "SKILLS", description: "扩展 AI 的专业工作流与工具能力，使用固定 GitHub 快照。", meta: "精选开源能力", icon: Sparkles, tone: "green" },
];

export function WorkbenchHome({ onOpenModule, onOpenModelSettings }: { onOpenModule: (module: WorkbenchModule) => void; onOpenModelSettings?: () => void }) {
  return (
    <section className="workbench-home" aria-label="超级工作台">
      <header className="workbench-home__heading">
        <div><span><WandSparkles size={14} /> SUPER WORKBENCH</span><h3>从这里开始创造</h3></div>
        <div className="workbench-home__actions">
          <p>分析项目、生成视觉内容，或为工作台安装新的专业能力。</p>
          {onOpenModelSettings && <button type="button" className="workbench-home__model-settings" onClick={onOpenModelSettings}><Settings2 size={14} aria-hidden="true" />配置模型</button>}
        </div>
      </header>
      <div className="workbench-home__grid">
        {MODULES.map(({ id, title, eyebrow, description, meta, icon: Icon, tone }) => (
          <button className={`workbench-launch workbench-launch--${tone}`} type="button" key={id} onClick={() => onOpenModule(id)}>
            <span className="workbench-launch__icon"><Icon size={20} /></span>
            <span className="workbench-launch__copy"><small>{eyebrow}</small><strong>{title}</strong><em>{description}</em></span>
            <span className="workbench-launch__meta">{meta}<b aria-hidden="true">→</b></span>
          </button>
        ))}
      </div>
    </section>
  );
}
