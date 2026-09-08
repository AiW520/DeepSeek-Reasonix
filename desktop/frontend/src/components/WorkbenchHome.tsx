import { ArrowUpRight, Blocks, BookOpen, BrainCircuit, Image, Presentation, Settings2, Sparkles } from "lucide-react";
import type { WorkbenchModule } from "../lib/workbench";
import "./WorkbenchHome.css";

const MODULES: Array<{ id: WorkbenchModule; title: string; description: string; meta: string; icon: typeof BrainCircuit; tone: string }> = [
  { id: "project-analysis", title: "项目深度分析", description: "扫描代码结构、技术栈与关键证据，建立可学习的项目地图。", meta: "本地只读分析", icon: BrainCircuit, tone: "teal" },
  { id: "image", title: "AI 图片创作", description: "生成产品图、海报、插画和透明背景素材，并保存到本地作品库。", meta: "Tuzi Coding", icon: Image, tone: "coral" },
  { id: "ppt", title: "PPT 智能制作", description: "从主题生成演示结构，逐页编辑内容并导出标准 PPTX 文件。", meta: "可编辑大纲", icon: Presentation, tone: "gold" },
  { id: "plugins", title: "插件广场", description: "安装工作、编程和视频插件，先做权限与运行时安全预检。", meta: "确认后安装", icon: Blocks, tone: "blue" },
  { id: "skills", title: "Skill 广场", description: "扩展 AI 的专业工作流与工具能力，使用固定 GitHub 快照。", meta: "精选开源能力", icon: Sparkles, tone: "green" },
  { id: "novel", title: "AI 小说工作室", description: "从世界观、人物到章节成稿，主 AI 与两位审查 AI 协作创作。", meta: "本地项目 · 可导出", icon: BookOpen, tone: "violet" },
];

export function WorkbenchHome({ onOpenModule, onOpenModelSettings }: { onOpenModule: (module: WorkbenchModule) => void; onOpenModelSettings?: () => void }) {
  return (
    <section className="workbench-home" aria-label="超级工作台">
      <header className="workbench-home__heading">
        <h3>工作台</h3>
        {onOpenModelSettings && <button type="button" className="workbench-home__model-settings" onClick={onOpenModelSettings}><Settings2 size={14} aria-hidden="true" />配置模型</button>}
      </header>
      <div className="workbench-home__grid">
        {MODULES.map(({ id, title, description, meta, icon: Icon, tone }) => (
          <button className={`workbench-launch workbench-launch--${tone}`} type="button" key={id} aria-label={title} onClick={() => onOpenModule(id)}>
            <span className="workbench-launch__icon"><Icon size={20} aria-hidden="true" /></span>
            <span className="workbench-launch__copy"><strong>{title}</strong><em>{description}</em></span>
            <span className="workbench-launch__meta">{meta}<ArrowUpRight size={15} aria-hidden="true" /></span>
          </button>
        ))}
      </div>
    </section>
  );
}
