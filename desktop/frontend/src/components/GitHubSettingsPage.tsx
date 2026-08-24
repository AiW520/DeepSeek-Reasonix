import { useCallback, useEffect, useRef, useState } from "react";
import { Check, Clipboard, ExternalLink, FolderGit2, GitBranch, Loader2, LockKeyhole, RefreshCw, Search, ShieldCheck, Star } from "lucide-react";
import { app, openExternal } from "../lib/bridge";
import { useI18n, type DictKey, type Locale } from "../lib/i18n";
import type { GitHubConnectionView, GitHubDeviceFlowStart, GitHubRepositoryView } from "../lib/types";
import "./GitHubSettingsPage.css";

const githubCopy = {
  en: { accountTitle: "GitHub workspace", accountDesc: "Connect without pasting a personal access token.", connectedAs: "Connected as @{login}", connect: "Connect GitHub", disconnect: "Disconnect", disconnectConfirm: "Remove this GitHub account from Reasonix?", keyringNote: "The OAuth token is deleted from the operating system credential vault.", notConfigured: "GitHub login is not configured in this build", notConfiguredDesc: "Set REASONIX_GITHUB_CLIENT_ID or inject an OAuth App client ID during release builds.", deviceInstruction: "Enter this one-time code on GitHub", openAuthorization: "Open authorization page", authDenied: "GitHub authorization was denied.", authExpired: "The GitHub authorization code expired.", securityTitle: "Protected connection", securityDesc: "The token stays in the system credential vault. Push, pull request, and merge remain separately approved actions.", searchPlaceholder: "Search repositories", refresh: "Refresh repositories", filterAll: "All", filterPublic: "Public", filterPrivate: "Private", repoCount: "{n} repositories", private: "Private", noDescription: "No description", noRepositories: "No repositories found.", clone: "Clone", cloneRepo: "Clone repository", cloneConfirm: "Clone into {path}. Existing folders are never overwritten.", clonedTo: "Repository cloned to {path}" },
  zh: { accountTitle: "GitHub 工作区", accountDesc: "无需粘贴个人访问令牌即可安全连接。", connectedAs: "已连接 @{login}", connect: "连接 GitHub", disconnect: "断开连接", disconnectConfirm: "确定从 Reasonix 移除此 GitHub 账号吗？", keyringNote: "OAuth Token 将从操作系统凭据库中删除。", notConfigured: "此构建尚未配置 GitHub 登录", notConfiguredDesc: "请设置 REASONIX_GITHUB_CLIENT_ID，或在正式发布构建时注入 OAuth App Client ID。", deviceInstruction: "在 GitHub 输入此一次性授权码", openAuthorization: "打开授权页面", authDenied: "GitHub 授权已被拒绝。", authExpired: "GitHub 授权码已过期。", securityTitle: "受保护的连接", securityDesc: "Token 只保存在系统凭据库。Push、创建 PR 和 Merge 仍需单独审批。", searchPlaceholder: "搜索仓库", refresh: "刷新仓库", filterAll: "全部", filterPublic: "公开", filterPrivate: "私有", repoCount: "{n} 个仓库", private: "私有", noDescription: "暂无描述", noRepositories: "没有找到仓库。", clone: "克隆", cloneRepo: "克隆仓库", cloneConfirm: "将克隆到 {path}，不会覆盖已有文件夹。", clonedTo: "仓库已克隆到 {path}" },
  "zh-TW": { accountTitle: "GitHub 工作區", accountDesc: "無需貼上個人存取權杖即可安全連接。", connectedAs: "已連接 @{login}", connect: "連接 GitHub", disconnect: "中斷連接", disconnectConfirm: "確定從 Reasonix 移除此 GitHub 帳號嗎？", keyringNote: "OAuth Token 將從作業系統憑證庫中刪除。", notConfigured: "此版本尚未配置 GitHub 登入", notConfiguredDesc: "請設定 REASONIX_GITHUB_CLIENT_ID，或在正式發佈版本中注入 OAuth App Client ID。", deviceInstruction: "在 GitHub 輸入此一次性授權碼", openAuthorization: "開啟授權頁面", authDenied: "GitHub 授權已被拒絕。", authExpired: "GitHub 授權碼已過期。", securityTitle: "受保護的連接", securityDesc: "Token 只保存在系統憑證庫。Push、建立 PR 和 Merge 仍需個別核准。", searchPlaceholder: "搜尋儲存庫", refresh: "重新整理儲存庫", filterAll: "全部", filterPublic: "公開", filterPrivate: "私人", repoCount: "{n} 個儲存庫", private: "私人", noDescription: "暫無描述", noRepositories: "沒有找到儲存庫。", clone: "複製", cloneRepo: "複製儲存庫", cloneConfirm: "將複製到 {path}，不會覆蓋現有資料夾。", clonedTo: "儲存庫已複製到 {path}" },
} satisfies Record<Locale, Record<string, string>>;

export function GitHubSettingsPage() {
  const { locale, t: globalT } = useI18n();
  const t = (key: string, vars?: Record<string, string | number>) => {
    const localKey = key.startsWith("github.") ? key.slice(7).replace(/\.([a-z])/g, (_, char: string) => char.toUpperCase()) : "";
    const localeCopy: Record<string, string> = githubCopy[locale];
    let value = localKey ? localeCopy[localKey] : globalT(key as DictKey, vars);
    for (const [name, replacement] of Object.entries(vars ?? {})) value = value.replaceAll(`{${name}}`, String(replacement));
    return value;
  };
  const [connection, setConnection] = useState<GitHubConnectionView | null>(null);
  const [flow, setFlow] = useState<GitHubDeviceFlowStart | null>(null);
  const [repos, setRepos] = useState<GitHubRepositoryView[]>([]);
  const [query, setQuery] = useState("");
  const [visibility, setVisibility] = useState("all");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [copied, setCopied] = useState(false);
  const [clonedPath, setClonedPath] = useState("");
  const cancelled = useRef(false);

  const loadConnection = useCallback(async () => {
    setError("");
    try { const next = await app.GitHubConnection(); setConnection(next); if (next.error) setError(next.error); } catch (err) { setError(String(err)); }
  }, []);

  const loadRepos = useCallback(async () => {
    if (!connection?.connected) return;
    setBusy("repos"); setError("");
    try { setRepos((await app.GitHubRepositories(query, visibility, "")).repositories ?? []); }
    catch (err) { setError(String(err)); } finally { setBusy(""); }
  }, [connection?.connected, query, visibility]);

  useEffect(() => { void loadConnection(); }, [loadConnection]);
  useEffect(() => {
    if (!connection?.connected) return;
    const timer = window.setTimeout(() => void loadRepos(), 250);
    return () => window.clearTimeout(timer);
  }, [connection?.connected, loadRepos]);
  useEffect(() => () => { cancelled.current = true; }, []);

  const startConnect = async () => {
    setBusy("connect"); setError(""); cancelled.current = false;
    try {
      const next = await app.StartGitHubDeviceFlow();
      setFlow(next); openExternal(next.verificationUri);
      let interval = Math.max(next.interval, 2);
      const expiresAt = Date.now() + next.expiresIn * 1000;
      while (!cancelled.current && Date.now() < expiresAt) {
        await new Promise((resolve) => window.setTimeout(resolve, interval * 1000));
        if (cancelled.current) break;
        const result = await app.PollGitHubDeviceFlow(next.deviceCode);
        if (result.status === "pending") { interval = Math.max(result.interval || interval, interval); continue; }
        if (result.status === "connected") { setConnection(result.connection); setFlow(null); break; }
        throw new Error(result.status === "access_denied" ? t("github.authDenied") : t("github.authExpired"));
      }
    } catch (err) { setError(String(err)); } finally { setBusy(""); }
  };

  const disconnect = async () => {
    if (!await app.ConfirmAction({ title: t("github.disconnect"), message: t("github.disconnectConfirm"), detail: t("github.keyringNote"), confirmLabel: t("github.disconnect"), cancelLabel: t("common.cancel"), destructive: true })) return;
    setBusy("disconnect");
    try { await app.DisconnectGitHub(); setRepos([]); setConnection(await app.GitHubConnection()); }
    catch (err) { setError(String(err)); } finally { setBusy(""); }
  };

  const clone = async (repo: GitHubRepositoryView) => {
    setClonedPath(""); setError("");
    const parent = await app.PickGitHubCloneParent();
    if (!parent) return;
    const target = `${parent.replace(/[\\/]$/, "")}\\${repo.name}`;
    if (!await app.ConfirmAction({ title: t("github.cloneRepo"), message: repo.fullName, detail: t("github.cloneConfirm", { path: target }), confirmLabel: t("github.clone"), cancelLabel: t("common.cancel"), destructive: false })) return;
    setBusy(`clone:${repo.id}`);
    try { setClonedPath((await app.CloneGitHubRepository(repo.owner, repo.name, parent)).path); }
    catch (err) { setError(String(err)); } finally { setBusy(""); }
  };

  if (!connection) return <div className="github-center__loading"><Loader2 className="spin" size={20} />{t("settings.loading")}</div>;
  return <div className="github-center">
    {error && <div className="banner banner--error" role="alert">{error}</div>}
    <section className="github-center__account">
      <div className="github-center__brand"><span className="github-center__logo"><GitBranch size={30} /></span><div><strong>{t("github.accountTitle")}</strong><span>{connection.connected ? t("github.connectedAs", { login: connection.login || "GitHub" }) : t("github.accountDesc")}</span></div></div>
      {connection.connected ? <div className="github-center__profile">{connection.avatarUrl && <img src={connection.avatarUrl} alt="" />}<div><b>{connection.name || connection.login}</b><span>@{connection.login}</span></div><button className="btn btn--small" onClick={() => void disconnect()} disabled={Boolean(busy)}>{t("github.disconnect")}</button></div> : <button className="btn btn--primary" onClick={() => void startConnect()} disabled={!connection.configured || Boolean(busy)}>{busy === "connect" ? <Loader2 className="spin" size={16} /> : <GitBranch size={16} />}{t("github.connect")}</button>}
    </section>
    {!connection.configured && <div className="github-center__setup"><LockKeyhole size={19} /><div><b>{t("github.notConfigured")}</b><span>{t("github.notConfiguredDesc")}</span></div></div>}
    {flow && <section className="github-center__device"><div><span>{t("github.deviceInstruction")}</span><button type="button" className="github-center__code" onClick={async () => { await navigator.clipboard.writeText(flow.userCode); setCopied(true); }}>{flow.userCode}{copied ? <Check size={16} /> : <Clipboard size={16} />}</button></div><div className="github-center__device-actions"><button className="btn" onClick={() => openExternal(flow.verificationUri)}><ExternalLink size={16} />{t("github.openAuthorization")}</button><button className="btn btn--ghost" onClick={() => { cancelled.current = true; setFlow(null); setBusy(""); }}>{t("common.cancel")}</button></div></section>}
    <div className="github-center__security"><ShieldCheck size={20} /><div><b>{t("github.securityTitle")}</b><span>{t("github.securityDesc")}</span></div>{connection.scopes && <code>{connection.scopes}</code>}</div>
    {connection.connected && <section className="github-center__repos">
      <div className="github-center__toolbar"><label><Search size={16} /><input value={query} onChange={(event) => setQuery(event.currentTarget.value)} onKeyDown={(event) => { if (event.key === "Enter") void loadRepos(); }} placeholder={t("github.searchPlaceholder")} /></label><div className="github-center__segments">{["all", "public", "private"].map((value) => <button key={value} className={visibility === value ? "active" : ""} onClick={() => setVisibility(value)}>{t(`github.filter.${value}` as any)}</button>)}</div><button className="icon-btn" title={t("github.refresh")} aria-label={t("github.refresh")} onClick={() => void loadRepos()}><RefreshCw size={17} className={busy === "repos" ? "spin" : ""} /></button></div>
      <div className="github-center__summary"><FolderGit2 size={16} />{t("github.repoCount", { n: repos.length })}</div>
      <div className="github-center__repo-list">{repos.map((repo) => <article className="github-repo" key={repo.id}><div className="github-repo__main"><div className="github-repo__title"><button onClick={() => openExternal(repo.htmlUrl)}>{repo.fullName}<ExternalLink size={13} /></button>{repo.private && <span className="github-repo__private">{t("github.private")}</span>}</div><p>{repo.description || t("github.noDescription")}</p><div className="github-repo__meta">{repo.language && <span><i />{repo.language}</span>}<span><Star size={13} />{repo.stars}</span><span>{repo.defaultBranch}</span><span>{new Date(repo.updatedAt).toLocaleDateString()}</span></div></div><button className="btn btn--small" disabled={Boolean(busy)} onClick={() => void clone(repo)}>{busy === `clone:${repo.id}` ? <Loader2 className="spin" size={15} /> : <FolderGit2 size={15} />}{t("github.clone")}</button></article>)}</div>
      {!repos.length && busy !== "repos" && <div className="empty">{t("github.noRepositories")}</div>}
      {clonedPath && <div className="banner banner--success">{t("github.clonedTo", { path: clonedPath })}</div>}
    </section>}
  </div>;
}
