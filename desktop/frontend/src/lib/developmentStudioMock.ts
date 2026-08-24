import type { DevelopmentStudioConfig, DevelopmentStudioEvent, DevelopmentStudioSnapshot, ModelInfo } from "./types";

const MODELS: ModelInfo[] = [
  { ref: "deepseek/deepseek-v4-flash", provider: "deepseek", model: "deepseek-v4-flash", current: true },
  { ref: "deepseek/deepseek-v4-pro", provider: "deepseek", model: "deepseek-v4-pro", current: false },
];

let paused = false;
let sequence = 8;
let config: DevelopmentStudioConfig = {
  enabled: true,
  teachingMode: "engineer",
  realtime: { enabled: true, model: MODELS[0].ref, effort: "high", maxSteps: 6, maxOutputTokens: 2200 },
  final: { enabled: true, model: MODELS[1].ref, effort: "xhigh", maxSteps: 10, maxOutputTokens: 4000 },
  minReviewIntervalMs: 4500,
  turnTokenBudget: 60000,
};

let events: DevelopmentStudioEvent[] = [
  { id: "dev-1", tabId: "tab_1", sequence: 1, at: Date.now() - 155000, role: "lead", kind: "turn_started", title: "主开发 AI 已开始本轮开发", detail: "正在理解需求并拆分实现边界。", why: "先确认目标和验证范围，避免后续返工。", status: "running" },
  { id: "dev-2", tabId: "tab_1", sequence: 2, at: Date.now() - 128000, role: "lead", kind: "activity", title: "正在读取工作台结构", detail: "定位右侧面板、桥接层和事件订阅接口。", why: "沿用现有架构可以降低长期维护成本。", target: "desktop/frontend/src/App.tsx", status: "passed", paths: ["desktop/frontend/src/App.tsx"] },
  { id: "dev-3", tabId: "tab_1", sequence: 3, at: Date.now() - 94000, role: "lead", kind: "change", title: "三 AI 协调器已接入", detail: "开发事件会持续进入结构化学习时间线。", why: "开发者能看到正在做什么、为什么做以及验证结果。", target: "desktop/development_studio_app.go", status: "passed", paths: ["desktop/development_studio_app.go"] },
  { id: "dev-4", tabId: "tab_1", sequence: 4, at: Date.now() - 73000, role: "realtime", kind: "status", title: "实时审查 AI 正在检查最新改动", detail: "只读检查当前差异、接口影响和最近失败证据。", status: "running", model: MODELS[0].ref },
  { id: "dev-5", tabId: "tab_1", sequence: 5, at: Date.now() - 51000, role: "realtime", kind: "finding", title: "实时审查 AI 已返回结论", detail: "建议：为协调器补充配置归一化、暂停取消和事件去重测试。当前未发现阻断问题。", severity: "suggestion", status: "reviewed", model: MODELS[0].ref, tokens: 1840, cost: 0.0086, currency: "¥" },
  { id: "dev-6", tabId: "tab_1", sequence: 6, at: Date.now() - 33000, role: "lead", kind: "test", title: "类型检查已完成", detail: "前端桥接签名和组件类型保持一致。", why: "类型检查可以在构建前发现跨层契约漂移。", status: "passed" },
  { id: "dev-7", tabId: "tab_1", sequence: 7, at: Date.now() - 16000, role: "final", kind: "status", title: "最终验收 AI 已启动", detail: "正在独立核对完整差异、测试证据与残留风险。", status: "running", model: MODELS[1].ref },
  { id: "dev-8", tabId: "tab_1", sequence: 8, at: Date.now() - 3000, role: "final", kind: "verdict", title: "最终验收 AI 已给出结论", detail: "条件通过：核心流程完整，两个审查角色保持只读。桌面壳集成后需补一次真实模型联调。", severity: "high", status: "warning", model: MODELS[1].ref, tokens: 3260, cost: 0.0164, currency: "¥" },
];

export const developmentStudioMock = {
  snapshot(tabId: string): DevelopmentStudioSnapshot {
    return {
      config: structuredClone(config),
      paused,
      lead: { role: "lead", state: "running", model: MODELS[1].ref, runs: 1, tokens: 12480, cost: 0.0412, currency: "¥" },
      realtime: { role: "realtime", state: "done", model: config.realtime.model, lastRun: Date.now() - 51000, runs: 3, tokens: 5140, cost: 0.0241, currency: "¥" },
      final: { role: "final", state: "done", model: config.final.model, lastRun: Date.now() - 3000, runs: 1, tokens: 3260, cost: 0.0164, currency: "¥" },
      events: events.map((event) => ({ ...event, tabId })),
      models: MODELS.map((model) => ({ ...model })),
    };
  },
  save(input: DevelopmentStudioConfig): void { config = structuredClone(input); },
  pause(tabId: string, next: boolean): DevelopmentStudioEvent {
    paused = next;
    const event: DevelopmentStudioEvent = { id: `dev-${++sequence}`, tabId, sequence, at: Date.now(), role: "system", kind: "status", title: next ? "三 AI 协作已暂停" : "三 AI 协作已恢复", status: next ? "paused" : "running" };
    events = [...events, event];
    return event;
  },
  clear(): void { events = []; },
  runFinal(tabId: string): DevelopmentStudioEvent {
    const event: DevelopmentStudioEvent = { id: `dev-${++sequence}`, tabId, sequence, at: Date.now(), role: "final", kind: "status", title: "最终验收 AI 已启动", detail: "正在进行只读独立验收。", status: "running", model: config.final.model };
    events = [...events, event];
    return event;
  },
};
