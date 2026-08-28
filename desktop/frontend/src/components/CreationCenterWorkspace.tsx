import { useEffect, useState } from "react";
import PptxGenJS from "pptxgenjs";
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  ExternalLink,
  FileImage,
  FileText,
  FolderOpen,
  Image as ImageIcon,
  Loader2,
  MonitorPlay,
  Palette,
  Presentation,
  RefreshCw,
  Sparkles,
  WandSparkles,
  X,
} from "lucide-react";
import { app } from "../lib/bridge";
import type {
  CreationImageRequest,
  CreationImageStatusView,
  CreationImageView,
  PresentationOutline,
  PresentationSlide,
} from "../lib/types";
import "./CreationCenterWorkspace.css";
import "./SuperWorkspace.css";

export type CreationCenterMode = "image" | "ppt";

const IMAGE_SIZES: Array<{ value: CreationImageRequest["size"]; label: string; hint: string }> = [
  { value: "1024x1024", label: "方形", hint: "1:1" },
  { value: "1536x1024", label: "横向", hint: "3:2" },
  { value: "1024x1536", label: "竖向", hint: "2:3" },
];

const DECK_THEMES = {
  graphite: { name: "石墨金", bg: "111317", panel: "1B1E24", text: "F4F2ED", muted: "A1A5AE", accent: "E8B44B", secondary: "3EC6B7" },
  daylight: { name: "日光蓝", bg: "F5F7FA", panel: "FFFFFF", text: "172033", muted: "64748B", accent: "2563EB", secondary: "0F9F8F" },
  forest: { name: "森林绿", bg: "0E1714", panel: "16231E", text: "EDF5F1", muted: "9BAEA5", accent: "55C596", secondary: "E5B75C" },
} as const;

type DeckThemeKey = keyof typeof DECK_THEMES;

function formatBytes(value: number): string {
  if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  return `${Math.max(1, Math.round(value / 1024))} KB`;
}

function safeFilename(value: string): string {
  const cleaned = value.trim().replace(/[<>:"/\\|?*\x00-\x1F]/g, "-").replace(/[. ]+$/g, "");
  return `${(cleaned || "Reasonix-Presentation").slice(0, 80)}.pptx`;
}

export function CreationCenterWorkspace({ mode, onModeChange, onClose }: { mode: CreationCenterMode; onModeChange: (mode: CreationCenterMode) => void; onClose: () => void }) {
  return (
    <section className="creation-center" aria-label="AI 创作中心">
      <header className="creation-center__header">
        <div className="creation-center__identity">
          <span className="creation-center__mark"><WandSparkles size={19} /></span>
          <div><span>REASONIX CREATIVE</span><h1>AI 创作中心</h1></div>
        </div>
        <nav className="creation-center__modes" aria-label="创作类型">
          <button type="button" className={mode === "image" ? "is-active" : ""} onClick={() => onModeChange("image")}><ImageIcon size={16} />图片创作</button>
          <button type="button" className={mode === "ppt" ? "is-active" : ""} onClick={() => onModeChange("ppt")}><Presentation size={16} />PPT 制作</button>
        </nav>
        <button className="creation-center__close" type="button" aria-label="关闭创作中心" onClick={onClose}><X size={18} /></button>
      </header>
      {mode === "image" ? <ImageStudio /> : <PresentationStudio />}
    </section>
  );
}

function ImageStudio() {
  const [status, setStatus] = useState<CreationImageStatusView | null>(null);
  const [request, setRequest] = useState<CreationImageRequest>({ prompt: "", size: "1536x1024", quality: "high", outputFormat: "png", background: "opaque" });
  const [images, setImages] = useState<CreationImageView[]>([]);
  const [selected, setSelected] = useState<CreationImageView | null>(null);
  const [preview, setPreview] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const refresh = async () => {
    const [nextStatus, recent] = await Promise.all([app.CreationImageStatus(), app.RecentCreationImages(12)]);
    setStatus(nextStatus);
    setImages(recent);
  };

  useEffect(() => { void refresh().catch((cause) => setError(String(cause))); }, []);

  const selectImage = async (image: CreationImageView) => {
    setSelected(image);
    setError("");
    if (image.preview) {
      setPreview(image.preview);
      return;
    }
    try { setPreview(await app.CreationImagePreview(image.id)); }
    catch (cause) { setError(cause instanceof Error ? cause.message : String(cause)); }
  };

  const generate = async () => {
    if (!request.prompt.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      const image = await app.GenerateCreationImage({ ...request, prompt: request.prompt.trim() });
      setSelected(image);
      setPreview(image.preview || await app.CreationImagePreview(image.id));
      setImages((current) => [image, ...current.filter((item) => item.id !== image.id)].slice(0, 12));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="image-studio">
      <aside className="image-studio__controls">
        <div className="studio-section-heading"><span>生成设置</span><small>固定使用 gpt-image-2</small></div>
        <label className="studio-field studio-field--prompt">
          <span>画面描述</span>
          <textarea value={request.prompt} onChange={(event) => setRequest((current) => ({ ...current, prompt: event.currentTarget.value }))} placeholder="描述主体、环境、构图、光线、材质和需要出现的文字..." maxLength={8000} />
          <small>{request.prompt.length}/8000</small>
        </label>
        <fieldset className="studio-fieldset">
          <legend>画布比例</legend>
          <div className="image-size-options">
            {IMAGE_SIZES.map((item) => <button type="button" key={item.value} className={request.size === item.value ? "is-active" : ""} onClick={() => setRequest((current) => ({ ...current, size: item.value }))}><span className={`ratio ratio--${item.hint.replace(":", "-")}`} /><strong>{item.label}</strong><small>{item.hint}</small></button>)}
          </div>
        </fieldset>
        <div className="studio-field-grid">
          <label className="studio-field"><span>质量</span><select value={request.quality} onChange={(event) => setRequest((current) => ({ ...current, quality: event.currentTarget.value as CreationImageRequest["quality"] }))}><option value="auto">自动</option><option value="medium">标准</option><option value="high">高清</option><option value="low">快速</option></select></label>
          <label className="studio-field"><span>格式</span><select value={request.outputFormat} onChange={(event) => setRequest((current) => ({ ...current, outputFormat: event.currentTarget.value as CreationImageRequest["outputFormat"] }))}><option value="png">PNG</option><option value="webp">WebP</option><option value="jpeg">JPEG</option></select></label>
        </div>
        <label className="studio-toggle"><input type="checkbox" checked={request.background === "transparent"} disabled={request.outputFormat === "jpeg"} onChange={(event) => setRequest((current) => ({ ...current, background: event.currentTarget.checked ? "transparent" : "opaque" }))} /><span><strong>透明背景</strong><small>适合图标、商品和素材抠图</small></span></label>
        {error && <div className="studio-error"><AlertTriangle size={15} />{error}</div>}
        <button className="studio-primary-action" type="button" disabled={busy || !request.prompt.trim() || status?.configured === false} onClick={() => void generate()}>{busy ? <Loader2 className="spin" size={17} /> : <Sparkles size={17} />}{busy ? "正在生成，勿重复提交" : "生成图片"}</button>
        <div className={`studio-connection${status?.configured ? " is-ready" : ""}`}><span />{status === null ? "正在检查图片服务" : status.configured ? "Tuzi Coding 已连接" : "尚未配置图片服务凭据"}</div>
      </aside>

      <main className="image-studio__canvas">
        <div className="studio-canvas-toolbar"><div><MonitorPlay size={15} /><span>作品预览</span></div>{selected && <div className="studio-canvas-actions"><button type="button" title="打开图片" onClick={() => void app.OpenLocalPath(selected.path)}><ExternalLink size={15} /></button><button type="button" title="在文件夹中显示" onClick={() => void app.RevealPath(selected.path)}><FolderOpen size={15} /></button><button type="button" title="另存为" onClick={() => void app.SaveLocalPathAs(selected.path)}><Download size={15} /></button></div>}</div>
        <div className={`studio-canvas-stage${busy ? " is-generating" : ""}`}>
          {preview ? <img src={preview} alt={selected?.prompt || "生成图片"} /> : <div className="studio-canvas-empty"><span><ImageIcon size={30} /></span><h2>把想法变成画面</h2><p>左侧填写描述，生成结果会自动保存在本地作品库。</p></div>}
          {busy && <div className="studio-generation-state"><Loader2 className="spin" size={24} /><strong>gpt-image-2 正在创作</strong><span>生成可能需要几十秒，本次不会自动重试</span></div>}
        </div>
        {selected && <footer className="studio-canvas-meta"><div><strong>{selected.filename}</strong><span>{selected.size || request.size} · {formatBytes(selected.bytes)}</span></div><p>{selected.prompt}</p></footer>}
      </main>

      <aside className="image-studio__library">
        <div className="studio-section-heading"><span>最近作品</span><button type="button" title="刷新作品库" onClick={() => void refresh()}><RefreshCw size={14} /></button></div>
        <div className="creation-library-list">
          {images.map((image) => <button type="button" key={image.id} className={selected?.id === image.id ? "is-active" : ""} onClick={() => void selectImage(image)}><span className="creation-library-thumb"><FileImage size={18} /></span><span><strong>{image.prompt || image.filename}</strong><small>{new Date(image.createdAt).toLocaleString()} · {formatBytes(image.bytes)}</small></span></button>)}
          {images.length === 0 && <div className="creation-library-empty"><ImageIcon size={21} /><span>生成的图片会显示在这里</span></div>}
        </div>
      </aside>
    </div>
  );
}

function PresentationStudio() {
  const [topic, setTopic] = useState("");
  const [audience, setAudience] = useState("");
  const [tone, setTone] = useState("专业、清晰、有说服力");
  const [slideCount, setSlideCount] = useState(8);
  const [theme, setTheme] = useState<DeckThemeKey>("graphite");
  const [outline, setOutline] = useState<PresentationOutline | null>(null);
  const [busy, setBusy] = useState<"draft" | "export" | "">("");
  const [message, setMessage] = useState("");

  const slides = outline?.slides ?? [];
  const deckTheme = DECK_THEMES[theme];

  const generateDraft = async () => {
    if (!topic.trim() || busy) return;
    setBusy("draft");
    setMessage("");
    try { setOutline(await app.DraftPresentation({ topic: topic.trim(), audience: audience.trim(), tone: tone.trim(), slideCount })); }
    catch (cause) { setMessage(cause instanceof Error ? cause.message : String(cause)); }
    finally { setBusy(""); }
  };

  const updateSlide = (index: number, patch: Partial<PresentationSlide>) => {
    setOutline((current) => current ? ({ ...current, slides: current.slides.map((slide, slideIndex) => slideIndex === index ? { ...slide, ...patch } : slide) }) : current);
  };

  const exportDeck = async () => {
    if (!outline || busy) return;
    setBusy("export");
    setMessage("");
    try {
      const pptx = buildPresentation(outline, theme);
      const payload = await pptx.write({ outputType: "base64", compression: true });
      if (typeof payload !== "string" || !payload) throw new Error("PPTX 生成结果为空");
      const path = await app.PickExportFile(safeFilename(outline.title), "application/vnd.openxmlformats-officedocument.presentationml.presentation");
      if (!path) return;
      await app.SaveExportFile(path, payload, true);
      setMessage(`已保存到 ${path}`);
    } catch (cause) {
      setMessage(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="ppt-studio">
      <aside className="ppt-studio__brief">
        <div className="studio-section-heading"><span>演示需求</span><small>由当前主模型生成大纲</small></div>
        <label className="studio-field"><span>演示主题</span><textarea className="ppt-topic" value={topic} onChange={(event) => setTopic(event.currentTarget.value)} placeholder="例如：2027 年 AI 产品战略与执行路线" maxLength={300} /></label>
        <label className="studio-field"><span>目标受众</span><input value={audience} onChange={(event) => setAudience(event.currentTarget.value)} placeholder="管理层、客户、投资人、团队..." /></label>
        <label className="studio-field"><span>表达风格</span><select value={tone} onChange={(event) => setTone(event.currentTarget.value)}><option>专业、清晰、有说服力</option><option>极简、数据驱动</option><option>故事化、富有感染力</option><option>教学型、循序渐进</option></select></label>
        <label className="studio-range"><span><strong>页面数量</strong><b>{slideCount} 页</b></span><input type="range" min={4} max={20} value={slideCount} onChange={(event) => setSlideCount(Number(event.currentTarget.value))} /></label>
        <fieldset className="studio-fieldset deck-theme-fieldset"><legend>视觉主题</legend><div className="deck-theme-options">{(Object.keys(DECK_THEMES) as DeckThemeKey[]).map((key) => <button type="button" key={key} className={theme === key ? "is-active" : ""} onClick={() => setTheme(key)}><span style={{ background: `linear-gradient(135deg,#${DECK_THEMES[key].bg} 0 55%,#${DECK_THEMES[key].accent} 55%)` }} /><strong>{DECK_THEMES[key].name}</strong></button>)}</div></fieldset>
        <button className="studio-primary-action" type="button" disabled={!topic.trim() || Boolean(busy)} onClick={() => void generateDraft()}>{busy === "draft" ? <Loader2 className="spin" size={17} /> : <Sparkles size={17} />}{outline ? "重新生成大纲" : "生成 PPT 大纲"}</button>
        <div className="ppt-process"><span className={outline ? "is-done" : "is-active"}><b>1</b>内容策划</span><i /><span className={outline ? "is-active" : ""}><b>2</b>编辑排版</span><i /><span><b>3</b>导出 PPTX</span></div>
      </aside>

      <main className="ppt-studio__editor">
        <header className="ppt-editor-header"><div><span>PRESENTATION EDITOR</span><h2>{outline?.title || "等待生成演示大纲"}</h2></div>{outline && <button className="ppt-export-button" type="button" disabled={Boolean(busy)} onClick={() => void exportDeck()}>{busy === "export" ? <Loader2 className="spin" size={16} /> : <Download size={16} />}导出 PPTX</button>}</header>
        {message && <div className={message.startsWith("已保存") ? "studio-success" : "studio-error"}>{message.startsWith("已保存") ? <CheckCircle2 size={15} /> : <AlertTriangle size={15} />}{message}</div>}
        {!outline ? <div className="ppt-empty"><span><Presentation size={31} /></span><h2>从主题到完整演示文稿</h2><p>输入主题和受众，AI 会建立叙事结构；生成后每页标题、要点与演讲备注都可以编辑。</p><div><FileText size={16} />结构化大纲<Palette size={16} />统一视觉<Download size={16} />标准 PPTX</div></div> : <div className="ppt-slide-list">{slides.map((slide, index) => <article className={`ppt-slide-row ppt-slide-row--${slide.layout || "content"}`} key={`${index}-${slide.title}`}><div className="ppt-slide-number">{String(index + 1).padStart(2, "0")}</div><div className="ppt-slide-mini" style={{ backgroundColor: `#${deckTheme.bg}`, color: `#${deckTheme.text}` }}><i style={{ backgroundColor: `#${deckTheme.accent}` }} /><strong>{slide.title || "无标题页面"}</strong><span>{(slide.bullets || []).slice(0, 3).map((bullet) => <em key={bullet}>{bullet}</em>)}</span></div><div className="ppt-slide-fields"><input value={slide.title} aria-label={`第 ${index + 1} 页标题`} onChange={(event) => updateSlide(index, { title: event.currentTarget.value })} /><textarea value={(slide.bullets || []).join("\n")} aria-label={`第 ${index + 1} 页要点`} onChange={(event) => updateSlide(index, { bullets: event.currentTarget.value.split("\n").map((item) => item.trim()).filter(Boolean).slice(0, 6) })} placeholder="每行一个要点" /><input className="ppt-notes" value={slide.speakerNotes || ""} aria-label={`第 ${index + 1} 页备注`} onChange={(event) => updateSlide(index, { speakerNotes: event.currentTarget.value })} placeholder="演讲备注（可选）" /></div></article>)}</div>}
      </main>
    </div>
  );
}

function buildPresentation(outline: PresentationOutline, themeKey: DeckThemeKey): PptxGenJS {
  const theme = DECK_THEMES[themeKey];
  const pptx = new PptxGenJS();
  pptx.layout = "LAYOUT_WIDE";
  pptx.author = "Reasonix";
  pptx.company = "Reasonix";
  pptx.subject = outline.subtitle || outline.title;
  pptx.title = outline.title;
  pptx.theme = { headFontFace: "Microsoft YaHei", bodyFontFace: "Microsoft YaHei" };

  outline.slides.forEach((item, index) => {
    const slide = pptx.addSlide();
    slide.background = { color: theme.bg };
    slide.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: 0.16, h: 7.5, fill: { color: theme.accent }, line: { color: theme.accent } });
    slide.addText("REASONIX", { x: 0.55, y: 0.35, w: 2.1, h: 0.25, fontFace: "Aptos", fontSize: 9, bold: true, color: theme.accent, charSpacing: 1.6, margin: 0 });
    slide.addText(String(index + 1).padStart(2, "0"), { x: 11.95, y: 0.35, w: 0.8, h: 0.3, fontSize: 9, color: theme.muted, align: "right", margin: 0 });
    const layout = item.layout || (index === 0 ? "cover" : "content");
    if (layout === "cover") {
      slide.addShape(pptx.ShapeType.rect, { x: 8.65, y: 0.95, w: 3.75, h: 4.95, fill: { color: theme.panel }, line: { color: theme.panel } });
      slide.addShape(pptx.ShapeType.arc, { x: 9.25, y: 1.5, w: 2.6, h: 2.6, rotate: 18, fill: { color: theme.accent, transparency: 28 }, line: { color: theme.secondary, transparency: 30, width: 2 } });
      slide.addText(outline.title || item.title, { x: 0.75, y: 2.05, w: 7.2, h: 1.55, fontSize: 29, bold: true, color: theme.text, margin: 0, breakLine: false, fit: "shrink" });
      slide.addText(outline.subtitle || item.subtitle || "", { x: 0.78, y: 3.85, w: 6.7, h: 0.72, fontSize: 13, color: theme.muted, margin: 0, fit: "shrink" });
      slide.addText(new Date().toLocaleDateString("zh-CN"), { x: 0.78, y: 6.35, w: 2.4, h: 0.3, fontSize: 9, color: theme.muted, margin: 0 });
    } else {
      slide.addText(item.title || `第 ${index + 1} 页`, { x: 0.75, y: 0.95, w: 10.9, h: 0.7, fontSize: 23, bold: true, color: theme.text, margin: 0, fit: "shrink" });
      if (item.subtitle) slide.addText(item.subtitle, { x: 0.78, y: 1.72, w: 10.4, h: 0.42, fontSize: 10.5, color: theme.muted, margin: 0, fit: "shrink" });
      const bullets = (item.bullets || []).filter(Boolean).slice(0, 6);
      const startY = item.subtitle ? 2.35 : 2.05;
      bullets.forEach((bullet, bulletIndex) => {
        const y = startY + bulletIndex * 0.72;
        slide.addShape(pptx.ShapeType.ellipse, { x: 0.8, y: y + 0.12, w: 0.13, h: 0.13, fill: { color: bulletIndex % 2 === 0 ? theme.accent : theme.secondary }, line: { color: bulletIndex % 2 === 0 ? theme.accent : theme.secondary } });
        slide.addText(bullet, { x: 1.12, y, w: 10.7, h: 0.42, fontSize: 15, color: theme.text, margin: 0, breakLine: false, fit: "shrink" });
      });
      slide.addShape(pptx.ShapeType.line, { x: 0.78, y: 6.72, w: 11.85, h: 0, line: { color: theme.panel, width: 1.1 } });
      slide.addText(outline.title, { x: 0.78, y: 6.88, w: 6.2, h: 0.25, fontSize: 8, color: theme.muted, margin: 0, fit: "shrink" });
    }
    if (item.speakerNotes) slide.addNotes(item.speakerNotes);
  });
  return pptx;
}
