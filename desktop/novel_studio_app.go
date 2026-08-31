package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/provider"
)

const (
	novelMaxTitleRunes   = 120
	novelMaxContentBytes = 4 * 1024 * 1024
	novelMaxProjectBytes = 12 * 1024 * 1024
)

var novelStoreMu sync.Mutex

type NovelWorldSetting struct {
	Era       string `json:"era"`
	Locations string `json:"locations"`
	Rules     string `json:"rules"`
	Themes    string `json:"themes"`
}

type NovelCharacter struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Traits string `json:"traits"`
	Arc    string `json:"arc"`
	Notes  string `json:"notes"`
}

type NovelOutline struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type NovelChapter struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Outline   string `json:"outline"`
	Content   string `json:"content"`
	Summary   string `json:"summary"`
	Status    string `json:"status"`
	Order     int    `json:"order"`
	WordCount int    `json:"wordCount"`
	UpdatedAt int64  `json:"updatedAt"`
}

type NovelProject struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Genre       string            `json:"genre"`
	Premise     string            `json:"premise"`
	Tone        string            `json:"tone"`
	TargetWords int               `json:"targetWords"`
	World       NovelWorldSetting `json:"world"`
	Characters  []NovelCharacter  `json:"characters"`
	Outlines    []NovelOutline    `json:"outlines"`
	Chapters    []NovelChapter    `json:"chapters"`
	CreatedAt   int64             `json:"createdAt"`
	UpdatedAt   int64             `json:"updatedAt"`
}

type NovelProjectSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Genre        string `json:"genre"`
	ChapterCount int    `json:"chapterCount"`
	WordCount    int    `json:"wordCount"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type NovelProjectInput struct {
	Title       string `json:"title"`
	Genre       string `json:"genre"`
	Premise     string `json:"premise"`
	Tone        string `json:"tone"`
	TargetWords int    `json:"targetWords"`
}

type NovelVersion struct {
	ID        string `json:"id"`
	ChapterID string `json:"chapterId"`
	Label     string `json:"label"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
	WordCount int    `json:"wordCount"`
}

type NovelAIRequest struct {
	ProjectID   string `json:"projectId"`
	ChapterID   string `json:"chapterId"`
	Action      string `json:"action"`
	Instruction string `json:"instruction"`
}

type NovelReviewIssue struct {
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion"`
}

type NovelAIResult struct {
	Content  string             `json:"content,omitempty"`
	Summary  string             `json:"summary,omitempty"`
	Outlines []NovelOutline     `json:"outlines,omitempty"`
	Issues   []NovelReviewIssue `json:"issues,omitempty"`
}

type NovelExportPayload struct {
	Filename      string `json:"filename"`
	Mime          string `json:"mime"`
	Payload       string `json:"payload"`
	Base64Encoded bool   `json:"base64Encoded"`
}

func novelsDir() string { return filepath.Join(config.ReasonixHomeDir(), "novels") }

func (a *App) ListNovelProjects() ([]NovelProjectSummary, error) {
	return listNovelProjectsAt(novelsDir())
}

func (a *App) CreateNovelProject(input NovelProjectInput) (NovelProject, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || utf8.RuneCountInString(input.Title) > novelMaxTitleRunes {
		return NovelProject{}, errors.New("小说名称不能为空且不能超过 120 个字符")
	}
	now := time.Now().UnixMilli()
	project := NovelProject{
		ID: novelID("novel"), Title: input.Title, Genre: trimRunes(input.Genre, 80),
		Premise: trimRunes(input.Premise, 4000), Tone: trimRunes(input.Tone, 300),
		TargetWords: input.TargetWords, Characters: []NovelCharacter{}, Outlines: []NovelOutline{}, Chapters: []NovelChapter{},
		CreatedAt: now, UpdatedAt: now,
	}
	if project.TargetWords < 1000 {
		project.TargetWords = 80000
	}
	if project.TargetWords > 10000000 {
		project.TargetWords = 10000000
	}
	if err := saveNovelProjectAt(novelsDir(), project, false); err != nil {
		return NovelProject{}, err
	}
	return project, nil
}

func (a *App) LoadNovelProject(id string) (NovelProject, error) {
	return loadNovelProjectAt(novelsDir(), id)
}

func (a *App) SaveNovelProject(project NovelProject) (NovelProject, error) {
	if err := saveNovelProjectAt(novelsDir(), project, true); err != nil {
		return NovelProject{}, err
	}
	return loadNovelProjectAt(novelsDir(), project.ID)
}

func (a *App) DeleteNovelProject(id string) error {
	if !validNovelID(id) {
		return errors.New("无效的小说项目 ID")
	}
	novelStoreMu.Lock()
	defer novelStoreMu.Unlock()
	return os.RemoveAll(filepath.Join(novelsDir(), id))
}

func (a *App) SaveNovelChapter(projectID string, chapter NovelChapter) (NovelChapter, error) {
	if len(chapter.Content) > novelMaxContentBytes {
		return NovelChapter{}, errors.New("章节正文超过 4 MB 限制")
	}
	project, err := loadNovelProjectAt(novelsDir(), projectID)
	if err != nil {
		return NovelChapter{}, err
	}
	chapter.Title = trimRunes(strings.TrimSpace(chapter.Title), novelMaxTitleRunes)
	if chapter.Title == "" {
		chapter.Title = fmt.Sprintf("第 %d 章", chapter.Order+1)
	}
	if !validNovelID(chapter.ID) {
		chapter.ID = novelID("chapter")
	}
	chapter.Status = normalizeNovelStatus(chapter.Status)
	chapter.WordCount = novelWordCount(chapter.Content)
	chapter.UpdatedAt = time.Now().UnixMilli()
	for i := range project.Chapters {
		if project.Chapters[i].ID == chapter.ID {
			if project.Chapters[i].Content != chapter.Content {
				_ = saveNovelVersionAt(novelsDir(), projectID, project.Chapters[i], "自动保存前")
			}
			project.Chapters[i] = chapter
			if err := saveNovelProjectAt(novelsDir(), project, true); err != nil {
				return NovelChapter{}, err
			}
			return chapter, nil
		}
	}
	project.Chapters = append(project.Chapters, chapter)
	if err := saveNovelProjectAt(novelsDir(), project, true); err != nil {
		return NovelChapter{}, err
	}
	return chapter, nil
}

func (a *App) ListNovelVersions(projectID, chapterID string) ([]NovelVersion, error) {
	if !validNovelID(projectID) || !validNovelID(chapterID) {
		return nil, errors.New("无效的项目或章节 ID")
	}
	dir := filepath.Join(novelsDir(), projectID, "versions", chapterID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []NovelVersion{}, nil
	}
	if err != nil {
		return nil, err
	}
	versions := make([]NovelVersion, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil || len(data) > novelMaxContentBytes+64*1024 {
			continue
		}
		var version NovelVersion
		if json.Unmarshal(data, &version) == nil {
			versions = append(versions, version)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].CreatedAt > versions[j].CreatedAt })
	return versions, nil
}

func (a *App) RestoreNovelVersion(projectID, chapterID, versionID string) (NovelChapter, error) {
	if !validNovelID(projectID) || !validNovelID(chapterID) || !validNovelID(versionID) {
		return NovelChapter{}, errors.New("无效的版本 ID")
	}
	data, err := os.ReadFile(filepath.Join(novelsDir(), projectID, "versions", chapterID, versionID+".json"))
	if err != nil {
		return NovelChapter{}, err
	}
	var version NovelVersion
	if err := json.Unmarshal(data, &version); err != nil {
		return NovelChapter{}, err
	}
	project, err := loadNovelProjectAt(novelsDir(), projectID)
	if err != nil {
		return NovelChapter{}, err
	}
	for _, chapter := range project.Chapters {
		if chapter.ID == chapterID {
			chapter.Content = version.Content
			return a.SaveNovelChapter(projectID, chapter)
		}
	}
	return NovelChapter{}, errors.New("章节不存在")
}

func (a *App) GenerateNovelContent(input NovelAIRequest) (NovelAIResult, error) {
	project, err := loadNovelProjectAt(novelsDir(), input.ProjectID)
	if err != nil {
		return NovelAIResult{}, err
	}
	input.Action = strings.TrimSpace(input.Action)
	if !validNovelAction(input.Action) {
		return NovelAIResult{}, errors.New("不支持的小说 AI 操作")
	}
	chapter := NovelChapter{}
	if input.ChapterID != "" {
		for _, item := range project.Chapters {
			if item.ID == input.ChapterID {
				chapter = item
				break
			}
		}
	}
	if input.Action != "outline" && chapter.ID == "" {
		return NovelAIResult{}, errors.New("请先选择章节")
	}
	prompt, system, err := novelPrompt(project, chapter, input)
	if err != nil {
		return NovelAIResult{}, err
	}
	raw, err := a.runNovelModel(system, prompt)
	if err != nil {
		return NovelAIResult{}, err
	}
	return parseNovelAIResult(raw, input.Action)
}

func (a *App) ExportNovelProject(projectID, format string) (NovelExportPayload, error) {
	project, err := loadNovelProjectAt(novelsDir(), projectID)
	if err != nil {
		return NovelExportPayload{}, err
	}
	format = strings.ToLower(strings.TrimSpace(format))
	base := safeNovelFilename(project.Title)
	switch format {
	case "md":
		return NovelExportPayload{Filename: base + ".md", Mime: "text/markdown", Payload: novelMarkdown(project)}, nil
	case "txt":
		return NovelExportPayload{Filename: base + ".txt", Mime: "text/plain", Payload: novelPlainText(project)}, nil
	case "docx":
		data, buildErr := buildNovelDOCX(project)
		if buildErr != nil {
			return NovelExportPayload{}, buildErr
		}
		return NovelExportPayload{Filename: base + ".docx", Mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Payload: base64.StdEncoding.EncodeToString(data), Base64Encoded: true}, nil
	default:
		return NovelExportPayload{}, errors.New("导出格式仅支持 Markdown、TXT 或 DOCX")
	}
}

func (a *App) runNovelModel(system, prompt string) (string, error) {
	return a.runNovelModelContext(a.reqCtx(), system, prompt)
}

func (a *App) runNovelModelContext(parent context.Context, system, prompt string) (string, error) {
	cfg, err := config.LoadForRoot(a.activeWorkspaceRoot())
	if err != nil {
		return "", fmt.Errorf("读取模型配置: %w", err)
	}
	resolved, _, ok := cfg.ResolveModelWithFallback(cfg.DefaultModel)
	if !ok {
		return "", errors.New("当前没有可用的默认模型，请先配置模型")
	}
	entry, ok := cfg.ResolveModel(resolved)
	if !ok {
		return "", errors.New("无法解析默认模型")
	}
	prov, err := boot.NewProviderWithProxy(entry, cfg.NetworkProxySpec())
	if err != nil {
		return "", fmt.Errorf("初始化小说模型: %w", err)
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 180*time.Second)
	defer cancel()
	stream, err := prov.Stream(ctx, provider.Request{Messages: []provider.Message{{Role: provider.RoleSystem, Content: system}, {Role: provider.RoleUser, Content: prompt}}, MaxTokens: 12000})
	if err != nil {
		return "", fmt.Errorf("请求小说模型: %w", err)
	}
	var output strings.Builder
	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("小说生成超时: %w", ctx.Err())
		case chunk, open := <-stream:
			if !open {
				return output.String(), nil
			}
			switch chunk.Type {
			case provider.ChunkText:
				if output.Len()+len(chunk.Text) > novelMaxContentBytes {
					return "", errors.New("模型响应超过 4 MB 限制")
				}
				output.WriteString(chunk.Text)
			case provider.ChunkError:
				return "", fmt.Errorf("小说生成失败: %w", chunk.Err)
			case provider.ChunkDone:
				return output.String(), nil
			}
		}
	}
}

func listNovelProjectsAt(root string) ([]NovelProjectSummary, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []NovelProjectSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]NovelProjectSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validNovelID(entry.Name()) {
			continue
		}
		project, loadErr := loadNovelProjectAt(root, entry.Name())
		if loadErr != nil {
			continue
		}
		total := 0
		for _, chapter := range project.Chapters {
			total += chapter.WordCount
		}
		result = append(result, NovelProjectSummary{ID: project.ID, Title: project.Title, Genre: project.Genre, ChapterCount: len(project.Chapters), WordCount: total, UpdatedAt: project.UpdatedAt})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result, nil
}

func loadNovelProjectAt(root, id string) (NovelProject, error) {
	if !validNovelID(id) {
		return NovelProject{}, errors.New("无效的小说项目 ID")
	}
	data, err := os.ReadFile(filepath.Join(root, id, "project.json"))
	if err != nil {
		return NovelProject{}, err
	}
	if len(data) > novelMaxProjectBytes {
		return NovelProject{}, errors.New("小说项目文件超过大小限制")
	}
	var project NovelProject
	if err := json.Unmarshal(data, &project); err != nil {
		return NovelProject{}, fmt.Errorf("解析小说项目: %w", err)
	}
	if project.Characters == nil {
		project.Characters = []NovelCharacter{}
	}
	if project.Outlines == nil {
		project.Outlines = []NovelOutline{}
	}
	if project.Chapters == nil {
		project.Chapters = []NovelChapter{}
	}
	return project, nil
}

func saveNovelProjectAt(root string, project NovelProject, requireExisting bool) error {
	if !validNovelID(project.ID) {
		return errors.New("无效的小说项目 ID")
	}
	project.Title = strings.TrimSpace(project.Title)
	if project.Title == "" || utf8.RuneCountInString(project.Title) > novelMaxTitleRunes {
		return errors.New("小说名称不能为空且不能超过 120 个字符")
	}
	if requireExisting {
		if _, err := os.Stat(filepath.Join(root, project.ID, "project.json")); err != nil {
			return err
		}
	}
	project.UpdatedAt = time.Now().UnixMilli()
	for i := range project.Chapters {
		if len(project.Chapters[i].Content) > novelMaxContentBytes {
			return errors.New("章节正文超过 4 MB 限制")
		}
		project.Chapters[i].WordCount = novelWordCount(project.Chapters[i].Content)
		project.Chapters[i].Status = normalizeNovelStatus(project.Chapters[i].Status)
	}
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > novelMaxProjectBytes {
		return errors.New("小说项目超过 12 MB 限制，请拆分项目")
	}
	novelStoreMu.Lock()
	defer novelStoreMu.Unlock()
	dir := filepath.Join(root, project.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "project.json"), data, 0o600)
}

func saveNovelVersionAt(root, projectID string, chapter NovelChapter, label string) error {
	if chapter.Content == "" || !validNovelID(projectID) || !validNovelID(chapter.ID) {
		return nil
	}
	version := NovelVersion{ID: novelID("version"), ChapterID: chapter.ID, Label: label, Content: chapter.Content, CreatedAt: time.Now().UnixMilli(), WordCount: novelWordCount(chapter.Content)}
	data, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(root, projectID, "versions", chapter.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, version.ID+".json"), data, 0o600)
}

func novelPrompt(project NovelProject, chapter NovelChapter, input NovelAIRequest) (string, string, error) {
	type chapterIndex struct {
		Order   int    `json:"order"`
		Title   string `json:"title"`
		Summary string `json:"summary"`
	}
	indexes := make([]chapterIndex, 0, len(project.Chapters))
	for _, item := range project.Chapters {
		if item.ID != chapter.ID && strings.TrimSpace(item.Summary) != "" {
			indexes = append(indexes, chapterIndex{Order: item.Order, Title: item.Title, Summary: trimRunes(item.Summary, 500)})
		}
	}
	if len(indexes) > 12 {
		indexes = indexes[len(indexes)-12:]
	}
	memory, _ := loadNovelMemory(project.ID)
	memory = compactNovelMemoryForPrompt(memory)
	contextJSON, _ := json.Marshal(struct {
		Title, Genre, Premise, Tone string
		World                       NovelWorldSetting
		Characters                  []NovelCharacter
		Outlines                    []NovelOutline
		ChapterIndex                []chapterIndex
		LongMemory                  NovelLongMemory
	}{project.Title, project.Genre, project.Premise, project.Tone, project.World, project.Characters, project.Outlines, indexes, memory})
	contextText := string(contextJSON)
	if len(contextText) > 96*1024 {
		contextText = trimRunes(contextText, 64000)
	}
	chapterContent := chapter.Content
	if runes := []rune(chapterContent); len(runes) > 60000 {
		chapterContent = "[正文前部已由章节摘要压缩]\n" + string(runes[len(runes)-60000:])
	}
	instruction := trimRunes(input.Instruction, 2000)
	base := fmt.Sprintf("项目资料（含最近章节摘要索引）：%s\n当前章节：%s\n章节大纲：%s\n章节摘要：%s\n正文：\n%s\n\n用户补充要求：%s", contextText, chapter.Title, chapter.Outline, chapter.Summary, chapterContent, instruction)
	switch input.Action {
	case "outline":
		return "请根据项目资料规划 8-12 个章节。只输出 JSON：{\"outlines\":[{\"id\":\"outline-1\",\"title\":\"标题\",\"summary\":\"具体剧情和冲突\"}],\"summary\":\"总体结构说明\"}", "你是小说总编与故事架构师，擅长长篇结构、伏笔和人物弧光。严格输出 JSON，不要代码围栏。", nil
	case "review-live", "review-final":
		scope := "检查当前草稿的逻辑、人物一致性、节奏、视角和语言问题"
		if input.Action == "review-final" {
			scope = "对当前章节进行完稿级审查，尤其检查与项目设定和人物弧光的冲突"
		}
		return base + "\n任务：" + scope + "。只输出 JSON：{\"summary\":\"总体判断\",\"issues\":[{\"severity\":\"info|warning|critical\",\"category\":\"分类\",\"title\":\"问题标题\",\"detail\":\"证据\",\"suggestion\":\"可执行修改建议\"}]}", "你是独立小说审查编辑。不得续写正文，必须给出具体证据，严格输出 JSON。", nil
	default:
		actionText := map[string]string{"draft": "根据大纲从头创作本章", "continue": "从当前正文自然续写", "expand": "扩写正文并增加有效场景与细节", "rewrite": "按要求重写正文", "polish": "在不改变剧情事实的前提下润色正文"}[input.Action]
		return base + "\n任务：" + actionText + "。只输出 JSON：{\"content\":\"完整的本章正文\",\"summary\":\"150字以内章节摘要\"}。正文需要完整可读，不要解释创作过程。", "你是主笔小说家。尊重既有设定，保持人物声音与叙事视角一致，严格输出 JSON，不要代码围栏。", nil
	}
}

func parseNovelAIResult(raw, action string) (NovelAIResult, error) {
	raw = strings.TrimSpace(raw)
	if start := strings.Index(raw, "{"); start >= 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 {
		raw = raw[:end+1]
	}
	var result NovelAIResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return NovelAIResult{}, fmt.Errorf("解析小说模型响应: %w", err)
	}
	if action == "outline" && len(result.Outlines) == 0 {
		return NovelAIResult{}, errors.New("模型未返回章节大纲")
	}
	if strings.HasPrefix(action, "review") && result.Issues == nil {
		result.Issues = []NovelReviewIssue{}
	}
	if action != "outline" && !strings.HasPrefix(action, "review") && strings.TrimSpace(result.Content) == "" {
		return NovelAIResult{}, errors.New("模型未返回章节正文")
	}
	return result, nil
}

func validNovelAction(action string) bool {
	switch action {
	case "outline", "draft", "continue", "expand", "rewrite", "polish", "review-live", "review-final":
		return true
	}
	return false
}

func validNovelID(id string) bool {
	if id == "" || len(id) > 96 {
		return false
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func novelID(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }
func trimRunes(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}
func novelWordCount(value string) int { return utf8.RuneCountInString(strings.TrimSpace(value)) }
func normalizeNovelStatus(value string) string {
	if value == "review" || value == "done" {
		return value
	}
	return "draft"
}
func safeNovelFilename(value string) string {
	value = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\\|?*`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(value))
	value = strings.Trim(value, ". ")
	if value == "" {
		value = "Reasonix-Novel"
	}
	return trimRunes(value, 80)
}

func novelMarkdown(project NovelProject) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n> %s\n\n", project.Title, project.Premise)
	chapters := append([]NovelChapter(nil), project.Chapters...)
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Order < chapters[j].Order })
	for _, chapter := range chapters {
		fmt.Fprintf(&out, "## %s\n\n%s\n\n", chapter.Title, chapter.Content)
	}
	return out.String()
}

func novelPlainText(project NovelProject) string {
	text := novelMarkdown(project)
	text = strings.ReplaceAll(text, "# ", "")
	text = strings.ReplaceAll(text, "## ", "")
	text = strings.ReplaceAll(text, "> ", "")
	return text
}

func buildNovelDOCX(project NovelProject) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
	}
	var body strings.Builder
	paragraph := func(text, style string) {
		fmt.Fprintf(&body, `<w:p><w:pPr><w:pStyle w:val="%s"/></w:pPr><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, style, html.EscapeString(text))
	}
	paragraph(project.Title, "Title")
	if project.Premise != "" {
		paragraph(project.Premise, "Subtitle")
	}
	chapters := append([]NovelChapter(nil), project.Chapters...)
	sort.SliceStable(chapters, func(i, j int) bool { return chapters[i].Order < chapters[j].Order })
	for _, chapter := range chapters {
		paragraph(chapter.Title, "Heading1")
		for _, line := range strings.Split(strings.ReplaceAll(chapter.Content, "\r\n", "\n"), "\n") {
			if strings.TrimSpace(line) != "" {
				paragraph(line, "Normal")
			}
		}
	}
	files["word/document.xml"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr></w:body></w:document>`
	for name, content := range files {
		writer, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
