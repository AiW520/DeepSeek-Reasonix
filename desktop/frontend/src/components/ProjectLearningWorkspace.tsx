import { useEffect, useState, type ReactElement } from "react";
import {
  AlertTriangle,
  ArrowRight,
  BookOpenCheck,
  Boxes,
  Braces,
  CheckCircle2,
  ChevronRight,
  CircleStop,
  Code2,
  Database,
  FileCode2,
  FileSearch,
  FolderOpen,
  GitBranch,
  GraduationCap,
  Layers3,
  Network,
  Play,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { app } from "../lib/bridge";
import type { ProjectAnalysisJobView, ProjectAnalysisResult } from "../lib/types";
import { ModalCloseButton } from "./ModalCloseButton";
import "./ProjectLearningWorkspace.css";
import "./SuperWorkspace.css";

type View = "overview" | "explorer" | "graph" | "security";

export function ProjectLearningWorkspace({ initialRoot = "", onClose, embedded = false }: { initialRoot?: string; onClose: () => void; embedded?: boolean }) {
  const [root, setRoot] = useState(initialRoot);
  const [job, setJob] = useState<ProjectAnalysisJobView | null>(null);
  const [view, setView] = useState<View>("overview");
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");

  const analyze = async (target = root) => {
    if (!target.trim()) return;
    setError("");
    try {
      const next = await app.StartProjectAnalysis(target);
      setJob(next);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  };

  useEffect(() => {
    if (typeof window !== "undefined" && !window.runtime && !job) void analyze(initialRoot || "~/projects/reasonix");
  // Browser preview loads a stable completed mock once.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!job || (job.state !== "queued" && job.state !== "running")) return;
    const timer = window.setInterval(() => {
      void app.ProjectAnalysisJob(job.id).then(setJob).catch((cause) => setError(String(cause)));
    }, 500);
    return () => window.clearInterval(timer);
  }, [job?.id, job?.state]);

  const result = job?.result;
  const running = job?.state === "queued" || job?.state === "running";
  const percent = job?.total ? Math.min(100, Math.round((job.done / job.total) * 100)) : 0;

  return (
    <div className={`learning-workspace${embedded ? " learning-workspace--embedded" : ""}`} role={embedded ? "region" : "dialog"} aria-modal={embedded ? undefined : true} aria-label="项目逆向学习引擎">
      <aside className="learning-workspace__rail">
        <div className="learning-workspace__brand">
          <span className="learning-workspace__brand-mark"><GraduationCap size={20} /></span>
          <span><strong>Project Atlas</strong><small>逆向学习引擎</small></span>
        </div>
        <nav className="learning-workspace__nav" aria-label="学习引擎视图">
          <RailButton active={view === "overview"} icon={<Layers3 />} label="项目总览" onClick={() => setView("overview")} />
          <RailButton active={view === "explorer"} icon={<FileSearch />} label="代码侦察" onClick={() => setView("explorer")} />
          <RailButton active={view === "graph"} icon={<Network />} label="依赖证据" onClick={() => setView("graph")} />
          <RailButton active={view === "security"} icon={<ShieldCheck />} label="安全预检" onClick={() => setView("security")} />
        </nav>
        <div className="learning-workspace__roadmap">
          <span>学习进度</span>
          <strong>侦察阶段</strong>
          <div><i style={{ width: result ? "24%" : "4%" }} /></div>
          <small>架构契约已建立，解析、图谱与学习模块按阶段开放</small>
        </div>
        <div className="learning-workspace__rail-foot">
          <ShieldCheck size={15} /> 本地分析 · 证据优先
        </div>
      </aside>

      <main className="learning-workspace__main">
        <header className="learning-workspace__topbar">
          <div>
            <span className="learning-workspace__eyebrow">PROJECT INTELLIGENCE</span>
            <h1>{result?.name || "项目深度逆向学习"}</h1>
          </div>
          <div className="learning-workspace__top-actions">
            {result && <span className="learning-workspace__freshness"><CheckCircle2 size={14} /> 已建立证据索引</span>}
            <ModalCloseButton label="关闭项目学习工作台" onClick={onClose} />
          </div>
        </header>

        <section className="learning-workspace__commandbar">
          <div className="learning-workspace__path">
            <FolderOpen size={17} />
            <input value={root} onChange={(event) => setRoot(event.currentTarget.value)} placeholder="选择本地项目或 Git 仓库" aria-label="项目目录" />
          </div>
          <button className="atlas-btn atlas-btn--secondary" type="button" onClick={async () => { const selected = await app.PickProjectAnalysisRoot(); if (selected) setRoot(selected); }}>
            <FolderOpen size={16} /> 选择项目
          </button>
          {running ? (
            <button className="atlas-btn atlas-btn--danger" type="button" onClick={() => job && void app.CancelProjectAnalysis(job.id)}><CircleStop size={16} /> 停止</button>
          ) : (
            <button className="atlas-btn atlas-btn--primary" type="button" disabled={!root.trim()} onClick={() => void analyze()}><Play size={16} /> {result ? "重新分析" : "开始侦察"}</button>
          )}
        </section>

        {error && <div className="learning-workspace__error"><AlertTriangle size={16} />{error}</div>}
        {running && <AnalysisProgress job={job} percent={percent} />}
        {!result && !running ? <EmptyProject /> : result ? (
          <div className="learning-workspace__content">
            {view === "overview" && <Overview result={result} />}
            {view === "explorer" && <Explorer result={result} query={query} setQuery={setQuery} />}
            {view === "graph" && <EvidenceGraph result={result} />}
            {view === "security" && <Security result={result} />}
          </div>
        ) : null}
      </main>
    </div>
  );
}

function RailButton({ active, icon, label, onClick }: { active: boolean; icon: ReactElement<{ size?: number }>; label: string; onClick: () => void }) {
  return <button className={active ? "active" : ""} type="button" onClick={onClick}>{icon}<span>{label}</span><ChevronRight size={14} /></button>;
}

function AnalysisProgress({ job, percent }: { job: ProjectAnalysisJobView; percent: number }) {
  return <div className="atlas-progress"><span className="atlas-progress__pulse"><RefreshCw size={18} /></span><div><strong>{job.phase}</strong><small>{job.done.toLocaleString()} / {job.total.toLocaleString()} 项</small><div className="atlas-progress__track"><i style={{ width: `${percent}%` }} /></div></div><b>{percent}%</b></div>;
}

function EmptyProject() {
  return <div className="atlas-empty"><span><Boxes size={34} /></span><h2>把一个真实项目变成你的学习地图</h2><p>系统只读取结构、声明与依赖证据，不会运行仓库脚本，也不会读取密钥内容。</p><div><ShieldCheck size={15} /> 敏感文件隔离 <GitBranch size={15} /> Git 友好 <Database size={15} /> 本地投影</div></div>;
}

function Overview({ result }: { result: ProjectAnalysisResult }) {
  const topLanguages = languageBreakdown(result);
  return <>
    <div className="atlas-stat-grid">
      <Metric icon={<FileCode2 />} label="已索引文件" value={result.files.length.toLocaleString()} meta={`${result.files.reduce((sum, file) => sum + file.lines, 0).toLocaleString()} 行代码`} />
      <Metric icon={<Braces />} label={result.parse ? "正式语法节点" : "代码符号"} value={(result.parse?.syntaxNodes ?? result.symbols.length).toLocaleString()} meta={result.parse ? `${result.parse.parsedFiles.toLocaleString()} 个 Go 文件 · 官方 AST` : "函数、类型与接口"} />
      <Metric icon={<Network />} label={result.dataFlow ? "真实数据流边" : result.callGraph ? "真实调用边" : result.resolution ? "跨文件引用" : "依赖证据"} value={(result.dataFlow?.flowEdges ?? result.callGraph?.callEdges ?? result.resolution?.localReferences ?? result.dependencies.length).toLocaleString()} meta={result.dataFlow ? `${result.dataFlow.definitions.toLocaleString()} 个定义 · 函数内 Go` : result.callGraph ? `${result.callGraph.functions.toLocaleString()} 个函数节点 · Go` : result.resolution ? `${result.resolution.localPackages.toLocaleString()} 个 Go 包 · 已解析` : "可回溯到源文件"} />
      <Metric icon={<ShieldCheck />} label="安全隔离" value={result.sensitiveFiles.length.toString()} meta="敏感文件未读取" tone="green" />
    </div>
    <div className="atlas-overview-grid">
      <section className="atlas-section atlas-section--stack">
        <div className="atlas-section__heading"><div><small>TECH STACK</small><h2>技术栈画像</h2></div><span>{result.packageManager || "未识别包管理器"}</span></div>
        <div className="atlas-stack-list">
          {topLanguages.map(({ name, count, percent }, index) => <div key={name}><span className={`atlas-stack-dot atlas-stack-dot--${index % 4}`} /><strong>{name}</strong><div><i style={{ width: `${percent}%` }} /></div><b>{count} 文件</b></div>)}
        </div>
        <div className="atlas-tags">{result.frameworks.map((item) => <span key={item}>{item}</span>)}</div>
      </section>
      <section className="atlas-section atlas-section--architecture">
        <div className="atlas-section__heading"><div><small>ARCHITECTURE SIGNAL</small><h2>系统结构线索</h2></div><span>{Math.round(averageConfidence(result) * 100)}% 平均置信度</span></div>
        <div className="atlas-architecture-flow">
          <ArchitectureNode icon={<Code2 />} label="客户端层" detail={result.languages.includes("TypeScript") ? "React / TypeScript" : result.languages[0] || "待识别"} />
          <ArrowRight />
          <ArchitectureNode icon={<Boxes />} label="业务核心" detail={`${result.symbols.length} 个符号`} />
          <ArrowRight />
          <ArchitectureNode icon={<Database />} label="数据与外部" detail={`${result.dependencies.length} 条依赖`} />
        </div>
        <div className="atlas-evidence-peek">{result.evidence.slice(0, 3).map((item) => <div key={item.id}><CheckCircle2 size={15} /><span><strong>{item.title}</strong><small>{item.sourceFile}:{item.line}</small></span><b>{Math.round(item.confidence * 100)}%</b></div>)}</div>
      </section>
    </div>
    <section className="atlas-section atlas-next-step"><span><Sparkles size={20} /></span><div><small>{result.dataFlow ? "GO DATA FLOW READY" : result.callGraph ? "GO CALL GRAPH READY" : result.resolution ? "SYMBOL RESOLUTION READY" : result.parse ? "GO AST READY" : "ARCHITECTURE BASELINE"}</small><h2>从核心入口和高连接模块开始阅读</h2><p>{result.dataFlow ? `Go 函数内数据流已完成，共建立 ${result.dataFlow.definitions.toLocaleString()} 个定义、${result.dataFlow.uses.toLocaleString()} 个使用和 ${result.dataFlow.flowEdges.toLocaleString()} 条可验证流向；指针别名、反射、并发时序与跨函数传播保持未知。` : result.callGraph ? `Go 静态调用图已完成，共建立 ${result.callGraph.functions.toLocaleString()} 个函数节点和 ${result.callGraph.callEdges.toLocaleString()} 条可验证调用边；动态分派保持未知。` : result.resolution ? `Go 跨文件解析已完成，共解析 ${result.resolution.resolvedSymbols.toLocaleString()} 个符号和 ${result.resolution.localReferences.toLocaleString()} 条引用；调用图、知识图谱与其他语言将在对应阶段开放。` : result.parse ? `Go 正式解析已完成，共建立 ${result.parse.syntaxNodes.toLocaleString()} 个语法节点；其他语言、调用图与知识图谱将按阶段开放。` : "项目侦察证据已接入版本化契约。AST、调用图、知识图谱与学习路径将在对应阶段逐步开放。"}</p></div><button type="button" disabled><BookOpenCheck size={16} /> 下一步：API 目录 <span>规划中</span></button></section>
  </>;
}

function Metric({ icon, label, value, meta, tone = "blue" }: { icon: React.ReactNode; label: string; value: string; meta: string; tone?: string }) {
  return <div className={`atlas-metric atlas-metric--${tone}`}><span>{icon}</span><div><small>{label}</small><strong>{value}</strong><p>{meta}</p></div></div>;
}

function ArchitectureNode({ icon, label, detail }: { icon: React.ReactNode; label: string; detail: string }) {
  return <div className="atlas-architecture-node"><span>{icon}</span><strong>{label}</strong><small>{detail}</small></div>;
}

function Explorer({ result, query, setQuery }: { result: ProjectAnalysisResult; query: string; setQuery: (value: string) => void }) {
  const normalized = query.trim().toLocaleLowerCase();
  const files = result.files.filter((file) => !file.sensitive && (!normalized || file.path.toLocaleLowerCase().includes(normalized) || file.language?.toLocaleLowerCase().includes(normalized))).slice(0, 200);
  const symbols = result.symbols.filter((symbol) => !normalized || `${symbol.name} ${symbol.kind} ${symbol.file}`.toLocaleLowerCase().includes(normalized)).slice(0, 200);
  return <div className="atlas-explorer">
    <div className="atlas-search"><Search size={16} /><input value={query} onChange={(event) => setQuery(event.currentTarget.value)} placeholder="搜索文件、语言或符号" /></div>
    <section className="atlas-section"><div className="atlas-section__heading"><div><small>FILES</small><h2>文件索引</h2></div><span>{files.length} / {result.files.length}</span></div><div className="atlas-table">{files.map((file) => <div key={file.path}><FileCode2 size={15} /><span><strong>{file.path}</strong><small>{file.language || "配置 / 文档"}</small></span><b>{file.lines.toLocaleString()} 行</b></div>)}</div></section>
    <section className="atlas-section"><div className="atlas-section__heading"><div><small>SYMBOLS</small><h2>符号索引</h2></div><span>{symbols.length} / {result.symbols.length}</span></div><div className="atlas-table">{symbols.map((symbol, index) => <div key={`${symbol.file}:${symbol.line}:${index}`}><Braces size={15} /><span><strong>{symbol.name}</strong><small>{symbol.file}:{symbol.line}</small></span><b>{symbol.kind}</b></div>)}</div></section>
  </div>;
}

function EvidenceGraph({ result }: { result: ProjectAnalysisResult }) {
  const nodes = result.languages.slice(0, 4);
  return <div className="atlas-graph-layout"><section className="atlas-section atlas-graph"><div className="atlas-section__heading"><div><small>DEPENDENCY MAP</small><h2>文件与技术栈关系</h2></div><span>{result.dependencies.length} 条边</span></div><div className="atlas-graph-canvas"><span className="atlas-graph-core"><Boxes size={22} /><b>{result.name}</b></span>{nodes.map((node, index) => <span key={node} className={`atlas-graph-node atlas-graph-node--${index}`}><Code2 size={16} />{node}</span>)}</div></section><section className="atlas-section"><div className="atlas-section__heading"><div><small>EVIDENCE</small><h2>可验证证据</h2></div></div><div className="atlas-evidence-list">{result.evidence.map((item) => <div key={item.id}><span>{Math.round(item.confidence * 100)}</span><div><strong>{item.title}</strong><p>{item.detail}</p><small>{item.sourceFile}:{item.line}</small></div></div>)}</div></section></div>;
}

function Security({ result }: { result: ProjectAnalysisResult }) {
  return <div className="atlas-security"><section className="atlas-security-hero"><span><ShieldCheck size={28} /></span><div><small>WORKSPACE SAFETY BOUNDARY</small><h2>本次分析遵循只读安全边界</h2><p>没有执行 install、build、test、Hook 或 MCP，也没有读取密钥和证书内容。</p></div><b>通过</b></section><div className="atlas-security-grid"><section className="atlas-section"><div className="atlas-section__heading"><div><small>PROTECTED</small><h2>敏感文件隔离</h2></div><span>{result.sensitiveFiles.length} 项</span></div><div className="atlas-table">{result.sensitiveFiles.length ? result.sensitiveFiles.map((path) => <div key={path}><ShieldCheck size={15} /><span><strong>{path}</strong><small>仅记录存在性，未读取内容</small></span><b>已隔离</b></div>) : <p className="atlas-section__empty">未发现敏感文件名。</p>}</div></section><section className="atlas-section"><div className="atlas-section__heading"><div><small>LIMITS</small><h2>扫描限制与失败项</h2></div><span>{result.skippedFiles} 跳过</span></div><div className="atlas-check-list"><div><CheckCircle2 />单文件最大 2 MB</div><div><CheckCircle2 />最多扫描 30,000 文件</div><div><CheckCircle2 />忽略依赖、构建和版本控制目录</div>{result.errors.map((item) => <div key={item}><AlertTriangle />{item}</div>)}</div></section></div></div>;
}

function languageBreakdown(result: ProjectAnalysisResult) {
  const counts = new Map<string, number>();
  result.files.forEach((file) => { if (file.language) counts.set(file.language, (counts.get(file.language) || 0) + 1); });
  const max = Math.max(1, ...counts.values());
  return [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5).map(([name, count]) => ({ name, count, percent: Math.max(8, Math.round((count / max) * 100)) }));
}

function averageConfidence(result: ProjectAnalysisResult) {
  return result.evidence.length ? result.evidence.reduce((sum, item) => sum + item.confidence, 0) / result.evidence.length : 0;
}
