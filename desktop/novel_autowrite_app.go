package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	novelBibleMaxBytes       = 8 * 1024 * 1024
	novelBibleTextMaxBytes   = 2 * 1024 * 1024
	novelMemoryMaxBytes      = 2 * 1024 * 1024
	novelJobMaxAttempts      = 3
	novelDefaultChapterWords = 2500
)

type NovelBible struct {
	Filename   string `json:"filename"`
	ImportedAt int64  `json:"importedAt"`
	Characters int    `json:"characters"`
	Summary    string `json:"summary"`
	Style      string `json:"style"`
}

type NovelChapterMemory struct {
	ChapterID string `json:"chapterId"`
	Order     int    `json:"order"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
}

type NovelLongMemory struct {
	Bible           NovelBible           `json:"bible"`
	GlobalSummary   string               `json:"globalSummary"`
	Timeline        []string             `json:"timeline"`
	CharacterStates map[string]string    `json:"characterStates"`
	Facts           []string             `json:"facts"`
	OpenThreads     []string             `json:"openThreads"`
	Constraints     []string             `json:"constraints"`
	Chapters        []NovelChapterMemory `json:"chapters"`
	UpdatedAt       int64                `json:"updatedAt"`
}

type NovelBibleImportResult struct {
	Bible  NovelBible      `json:"bible"`
	Memory NovelLongMemory `json:"memory"`
}

type NovelAutoWriteInput struct {
	ProjectID             string `json:"projectId"`
	TargetChapters        int    `json:"targetChapters"`
	TargetWordsPerChapter int    `json:"targetWordsPerChapter"`
	Instruction           string `json:"instruction"`
}

type NovelAutoWriteJob struct {
	ID                    string             `json:"id"`
	ProjectID             string             `json:"projectId"`
	Status                string             `json:"status"`
	Phase                 string             `json:"phase"`
	TargetChapters        int                `json:"targetChapters"`
	CompletedChapters     int                `json:"completedChapters"`
	TargetWordsPerChapter int                `json:"targetWordsPerChapter"`
	CurrentChapterTitle   string             `json:"currentChapterTitle"`
	Instruction           string             `json:"instruction"`
	LastError             string             `json:"lastError,omitempty"`
	LastIssues            []NovelReviewIssue `json:"lastIssues"`
	CreatedAt             int64              `json:"createdAt"`
	UpdatedAt             int64              `json:"updatedAt"`
}

type novelJobRuntime struct {
	cancel context.CancelFunc
}

type novelMemoryUpdate struct {
	GlobalSummary   string             `json:"globalSummary"`
	Timeline        []string           `json:"timeline"`
	CharacterStates map[string]string  `json:"characterStates"`
	Facts           []string           `json:"facts"`
	OpenThreads     []string           `json:"openThreads"`
	Issues          []NovelReviewIssue `json:"issues"`
}

func (a *App) ImportNovelBible(projectID string) (NovelBibleImportResult, error) {
	if !validNovelID(projectID) {
		return NovelBibleImportResult{}, errors.New("无效的小说项目 ID")
	}
	if _, err := loadNovelProjectAt(novelsDir(), projectID); err != nil {
		return NovelBibleImportResult{}, err
	}
	if a.ctx == nil {
		return NovelBibleImportResult{}, errors.New("请在桌面应用中导入作品圣经")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "选择作品圣经",
		DefaultDirectory: dialogDefaultDirectory(a.activeWorkspaceRoot()),
		Filters: []runtime.FileFilter{
			{DisplayName: "作品圣经 (*.md;*.txt;*.docx)", Pattern: "*.md;*.txt;*.docx"},
		},
	})
	if err != nil || path == "" {
		return NovelBibleImportResult{}, err
	}
	text, err := readNovelBible(path)
	if err != nil {
		return NovelBibleImportResult{}, err
	}
	project, err := loadNovelProjectAt(novelsDir(), projectID)
	if err != nil {
		return NovelBibleImportResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.bootContext(), 3*time.Minute)
	defer cancel()
	prompt := fmt.Sprintf("作品：%s\n类型：%s\n梗概：%s\n\n作品圣经：\n%s\n\n请提取可长期约束创作的资料。只输出 JSON：{\"summary\":\"核心设定摘要\",\"style\":\"文风要求\",\"timeline\":[\"时间线事件\"],\"characterStates\":{\"人物名\":\"初始状态与不可违背设定\"},\"facts\":[\"确定事实\"],\"openThreads\":[\"待展开主线\"],\"constraints\":[\"禁止违背的规则\"]}", project.Title, project.Genre, project.Premise, trimRunes(text, 180000))
	raw, err := a.runNovelModelContext(ctx, "你是长篇小说设定编辑。忠实提取资料，不添加作品圣经中不存在的事实，严格输出 JSON。", prompt)
	if err != nil {
		return NovelBibleImportResult{}, fmt.Errorf("分析作品圣经: %w", err)
	}
	var parsed struct {
		Summary         string            `json:"summary"`
		Style           string            `json:"style"`
		Timeline        []string          `json:"timeline"`
		CharacterStates map[string]string `json:"characterStates"`
		Facts           []string          `json:"facts"`
		OpenThreads     []string          `json:"openThreads"`
		Constraints     []string          `json:"constraints"`
	}
	if err := unmarshalNovelJSON(raw, &parsed); err != nil {
		return NovelBibleImportResult{}, fmt.Errorf("解析作品圣经分析结果: %w", err)
	}
	now := time.Now().UnixMilli()
	memory := NovelLongMemory{
		Bible:         NovelBible{Filename: filepath.Base(path), ImportedAt: now, Characters: utf8.RuneCountInString(text), Summary: trimRunes(parsed.Summary, 8000), Style: trimRunes(parsed.Style, 4000)},
		GlobalSummary: trimRunes(parsed.Summary, 8000), Timeline: limitNovelStrings(parsed.Timeline, 200, 500), CharacterStates: limitNovelMap(parsed.CharacterStates, 100, 1200), Facts: limitNovelStrings(parsed.Facts, 500, 500), OpenThreads: limitNovelStrings(parsed.OpenThreads, 200, 500), Constraints: limitNovelStrings(parsed.Constraints, 200, 500), Chapters: []NovelChapterMemory{}, UpdatedAt: now,
	}
	if err := saveNovelBibleSource(projectID, filepath.Ext(path), text); err != nil {
		return NovelBibleImportResult{}, err
	}
	if err := saveNovelMemory(projectID, memory); err != nil {
		return NovelBibleImportResult{}, err
	}
	return NovelBibleImportResult{Bible: memory.Bible, Memory: memory}, nil
}

func (a *App) NovelMemory(projectID string) (NovelLongMemory, error) {
	return loadNovelMemory(projectID)
}

func (a *App) StartNovelAutoWrite(input NovelAutoWriteInput) (NovelAutoWriteJob, error) {
	project, err := loadNovelProjectAt(novelsDir(), input.ProjectID)
	if err != nil {
		return NovelAutoWriteJob{}, err
	}
	if input.TargetChapters <= len(project.Chapters) {
		return NovelAutoWriteJob{}, errors.New("目标章节数必须大于当前章节数")
	}
	if input.TargetChapters > 1000 {
		return NovelAutoWriteJob{}, errors.New("单次任务最多规划到 1000 章")
	}
	if input.TargetWordsPerChapter < 500 {
		input.TargetWordsPerChapter = novelDefaultChapterWords
	}
	if input.TargetWordsPerChapter > 20000 {
		input.TargetWordsPerChapter = 20000
	}
	if existing, listErr := listNovelJobs(input.ProjectID); listErr == nil {
		for _, item := range existing {
			if item.Status == "running" || item.Status == "queued" || item.Status == "paused" {
				return NovelAutoWriteJob{}, errors.New("该小说已有未结束的自动写作任务")
			}
		}
	}
	now := time.Now().UnixMilli()
	job := NovelAutoWriteJob{ID: novelID("autowrite"), ProjectID: input.ProjectID, Status: "queued", Phase: "等待启动", TargetChapters: input.TargetChapters, CompletedChapters: len(project.Chapters), TargetWordsPerChapter: input.TargetWordsPerChapter, Instruction: trimRunes(input.Instruction, 4000), LastIssues: []NovelReviewIssue{}, CreatedAt: now, UpdatedAt: now}
	if err := saveNovelJob(job); err != nil {
		return NovelAutoWriteJob{}, err
	}
	a.launchNovelJob(job)
	return job, nil
}

func (a *App) ListNovelAutoWriteJobs(projectID string) ([]NovelAutoWriteJob, error) {
	if !validNovelID(projectID) {
		return nil, errors.New("无效的小说项目 ID")
	}
	return listNovelJobs(projectID)
}

func (a *App) PauseNovelAutoWrite(jobID string) error {
	return a.setNovelJobControl(jobID, "paused")
}

func (a *App) ResumeNovelAutoWrite(jobID string) error {
	job, err := findNovelJob(jobID)
	if err != nil {
		return err
	}
	if job.Status != "paused" && job.Status != "failed" && job.Status != "queued" {
		return errors.New("只有暂停、失败或排队中的任务可以继续")
	}
	job.Status, job.Phase, job.LastError = "queued", "准备继续", ""
	job.UpdatedAt = time.Now().UnixMilli()
	if err := saveNovelJob(job); err != nil {
		return err
	}
	a.launchNovelJob(job)
	return nil
}

func (a *App) StopNovelAutoWrite(jobID string) error {
	return a.setNovelJobControl(jobID, "stopped")
}

func (a *App) setNovelJobControl(jobID, status string) error {
	job, err := findNovelJob(jobID)
	if err != nil {
		return err
	}
	if job.Status == "completed" || job.Status == "stopped" {
		return nil
	}
	job.Status = status
	if status == "paused" {
		job.Phase = "已暂停"
	} else {
		job.Phase = "已停止"
	}
	job.UpdatedAt = time.Now().UnixMilli()
	if err := saveNovelJob(job); err != nil {
		return err
	}
	a.novelJobsMu.Lock()
	if active := a.novelJobs[jobID]; active != nil && active.cancel != nil {
		active.cancel()
	}
	a.novelJobsMu.Unlock()
	return nil
}

func (a *App) launchNovelJob(job NovelAutoWriteJob) {
	a.novelJobsMu.Lock()
	if _, exists := a.novelJobs[job.ID]; exists {
		a.novelJobsMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.bootContext())
	a.novelJobs[job.ID] = &novelJobRuntime{cancel: cancel}
	a.novelJobsMu.Unlock()
	go func() {
		defer func() {
			cancel()
			a.novelJobsMu.Lock()
			delete(a.novelJobs, job.ID)
			a.novelJobsMu.Unlock()
		}()
		a.runNovelAutoWrite(ctx, job.ID)
	}()
}

func (a *App) runNovelAutoWrite(ctx context.Context, jobID string) {
	job, err := findNovelJob(jobID)
	if err != nil {
		return
	}
	job.Status, job.Phase, job.UpdatedAt = "running", "读取长记忆", time.Now().UnixMilli()
	_ = saveNovelJob(job)
	for {
		if ctx.Err() != nil {
			a.finishCancelledNovelJob(jobID)
			return
		}
		job, err = findNovelJob(jobID)
		if err != nil || job.Status != "running" {
			return
		}
		project, loadErr := loadNovelProjectAt(novelsDir(), job.ProjectID)
		if loadErr != nil {
			a.failNovelJob(job, loadErr)
			return
		}
		if len(project.Chapters) >= job.TargetChapters {
			job.Status, job.Phase, job.CompletedChapters, job.UpdatedAt = "completed", "全部章节已完成", len(project.Chapters), time.Now().UnixMilli()
			_ = saveNovelJob(job)
			return
		}
		chapterNumber := len(project.Chapters) + 1
		outline := novelOutlineForChapter(project, chapterNumber)
		job.Phase = "主 AI 正在创作"
		job.CurrentChapterTitle = outline.Title
		job.UpdatedAt = time.Now().UnixMilli()
		_ = saveNovelJob(job)
		result, genErr := a.generateNovelContentContext(ctx, NovelAIRequest{ProjectID: job.ProjectID, Action: "draft", Instruction: fmt.Sprintf("这是自动连续写作的第 %d 章。目标约 %d 字。章节目标：%s。附加要求：%s", chapterNumber, job.TargetWordsPerChapter, outline.Summary, job.Instruction)}, NovelChapter{Title: outline.Title, Outline: outline.Summary, Order: chapterNumber})
		if genErr != nil {
			if ctx.Err() != nil {
				a.finishCancelledNovelJob(jobID)
			} else {
				a.failNovelJob(job, genErr)
			}
			return
		}
		chapter := NovelChapter{Title: outline.Title, Outline: outline.Summary, Content: result.Content, Summary: result.Summary, Status: "review", Order: chapterNumber}
		if strings.TrimSpace(chapter.Summary) == "" {
			chapter.Summary = trimRunes(chapter.Content, 500)
		}
		saved, saveErr := a.SaveNovelChapter(job.ProjectID, chapter)
		if saveErr != nil {
			a.failNovelJob(job, saveErr)
			return
		}
		job.Phase, job.CompletedChapters, job.UpdatedAt = "连续性 AI 正在更新长记忆", len(project.Chapters)+1, time.Now().UnixMilli()
		_ = saveNovelJob(job)
		memoryIssues, memoryErr := a.updateNovelMemoryFromChapter(ctx, project, saved)
		if memoryErr != nil {
			if ctx.Err() != nil {
				a.finishCancelledNovelJob(jobID)
				return
			}
			job.LastError = "长记忆更新失败，正文已安全保存：" + memoryErr.Error()
		}
		job.Phase, job.UpdatedAt = "完稿 AI 正在审查", time.Now().UnixMilli()
		_ = saveNovelJob(job)
		final, reviewErr := a.generateNovelContentContext(ctx, NovelAIRequest{ProjectID: job.ProjectID, ChapterID: saved.ID, Action: "review-final"}, NovelChapter{})
		if ctx.Err() != nil {
			a.finishCancelledNovelJob(jobID)
			return
		}
		job.LastIssues = append(memoryIssues, final.Issues...)
		if reviewErr != nil {
			job.LastError = "完稿审查失败，正文已安全保存：" + reviewErr.Error()
		}
		job.Phase, job.CurrentChapterTitle, job.CompletedChapters, job.UpdatedAt = "章节已保存，准备下一章", saved.Title, chapterNumber, time.Now().UnixMilli()
		_ = saveNovelJob(job)
	}
}

func (a *App) finishCancelledNovelJob(jobID string) {
	job, err := findNovelJob(jobID)
	if err != nil || job.Status == "paused" || job.Status == "stopped" {
		return
	}
	if a.shuttingDown.Load() {
		job.Status, job.Phase = "queued", "等待应用重启后恢复"
	} else {
		job.Status, job.Phase = "failed", "任务意外中断"
		job.LastError = "生成请求被取消"
	}
	job.UpdatedAt = time.Now().UnixMilli()
	_ = saveNovelJob(job)
}

func (a *App) failNovelJob(job NovelAutoWriteJob, err error) {
	job.Status, job.Phase, job.LastError, job.UpdatedAt = "failed", "任务失败", err.Error(), time.Now().UnixMilli()
	_ = saveNovelJob(job)
}

func (a *App) recoverNovelAutoWriteJobs() {
	a.novelRecoveryOnce.Do(func() {
		entries, err := os.ReadDir(novelsDir())
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() || !validNovelID(entry.Name()) {
				continue
			}
			jobs, listErr := listNovelJobs(entry.Name())
			if listErr != nil {
				continue
			}
			for _, job := range jobs {
				if job.Status == "running" || job.Status == "queued" {
					job.Status, job.Phase, job.UpdatedAt = "queued", "从断点恢复", time.Now().UnixMilli()
					_ = saveNovelJob(job)
					a.launchNovelJob(job)
				}
			}
		}
	})
}

func (a *App) cancelNovelAutoWriteJobs() {
	a.novelJobsMu.Lock()
	defer a.novelJobsMu.Unlock()
	for _, active := range a.novelJobs {
		if active != nil && active.cancel != nil {
			active.cancel()
		}
	}
}

func (a *App) generateNovelContentContext(ctx context.Context, input NovelAIRequest, explicitChapter NovelChapter) (NovelAIResult, error) {
	project, err := loadNovelProjectAt(novelsDir(), input.ProjectID)
	if err != nil {
		return NovelAIResult{}, err
	}
	chapter := explicitChapter
	if chapter.ID == "" && input.ChapterID != "" {
		for _, item := range project.Chapters {
			if item.ID == input.ChapterID {
				chapter = item
				break
			}
		}
	}
	prompt, system, err := novelPrompt(project, chapter, input)
	if err != nil {
		return NovelAIResult{}, err
	}
	raw, err := a.runNovelModelContext(ctx, system, prompt)
	if err != nil {
		return NovelAIResult{}, err
	}
	return parseNovelAIResult(raw, input.Action)
}

func (a *App) updateNovelMemoryFromChapter(ctx context.Context, project NovelProject, chapter NovelChapter) ([]NovelReviewIssue, error) {
	memory, _ := loadNovelMemory(project.ID)
	data, _ := json.Marshal(memory)
	prompt := fmt.Sprintf("已有长记忆：%s\n\n新章节标题：%s\n章节摘要：%s\n正文：%s\n\n更新长记忆并检查连续性。只输出 JSON：{\"globalSummary\":\"累计故事摘要\",\"timeline\":[\"关键事件\"],\"characterStates\":{\"人物\":\"最新状态\"},\"facts\":[\"已确立事实\"],\"openThreads\":[\"未回收伏笔\"],\"issues\":[{\"severity\":\"info|warning|critical\",\"category\":\"分类\",\"title\":\"问题\",\"detail\":\"证据\",\"suggestion\":\"建议\"}]}", string(data), chapter.Title, chapter.Summary, trimRunes(chapter.Content, 50000))
	raw, err := a.runNovelModelContext(ctx, "你是长篇小说连续性编辑和记忆管理员。压缩而不丢失关键事实，检查时间线、人物动机、伏笔和世界规则，严格输出 JSON。", prompt)
	if err != nil {
		return nil, err
	}
	var update novelMemoryUpdate
	if err := unmarshalNovelJSON(raw, &update); err != nil {
		return nil, err
	}
	memory.GlobalSummary = trimRunes(update.GlobalSummary, 12000)
	memory.Timeline = limitNovelStrings(update.Timeline, 500, 600)
	memory.CharacterStates = limitNovelMap(update.CharacterStates, 200, 1500)
	memory.Facts = limitNovelStrings(update.Facts, 1000, 600)
	memory.OpenThreads = limitNovelStrings(update.OpenThreads, 500, 600)
	memory.Chapters = append(memory.Chapters, NovelChapterMemory{ChapterID: chapter.ID, Order: chapter.Order, Title: chapter.Title, Summary: trimRunes(chapter.Summary, 1000)})
	if len(memory.Chapters) > 1000 {
		memory.Chapters = memory.Chapters[len(memory.Chapters)-1000:]
	}
	memory.UpdatedAt = time.Now().UnixMilli()
	return update.Issues, saveNovelMemory(project.ID, memory)
}

func novelOutlineForChapter(project NovelProject, number int) NovelOutline {
	index := number - 1
	if index >= 0 && index < len(project.Outlines) {
		outline := project.Outlines[index]
		if strings.TrimSpace(outline.Title) != "" {
			return outline
		}
	}
	return NovelOutline{ID: fmt.Sprintf("auto-outline-%d", number), Title: fmt.Sprintf("第 %d 章", number), Summary: "承接上一章，推进主线冲突并保持人物与世界设定一致"}
}

func readNovelBible(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > novelBibleMaxBytes {
		return "", errors.New("作品圣经不能超过 8 MB")
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".txt":
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", readErr
		}
		if len(data) > novelBibleTextMaxBytes {
			return "", errors.New("作品圣经文本不能超过 2 MB")
		}
		if !utf8.Valid(data) {
			return "", errors.New("作品圣经必须是 UTF-8 文本")
		}
		return strings.TrimSpace(string(data)), nil
	case ".docx":
		return readNovelDOCX(path)
	default:
		return "", errors.New("作品圣经仅支持 Markdown、TXT 或 DOCX")
	}
}

func readNovelDOCX(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("打开 DOCX: %w", err)
	}
	defer zr.Close()
	var totalUncompressed uint64
	if len(zr.File) > 256 {
		return "", errors.New("DOCX 包含过多文件")
	}
	for _, file := range zr.File {
		totalUncompressed += file.UncompressedSize64
		if totalUncompressed > novelBibleMaxBytes*8 {
			return "", errors.New("DOCX 解压内容总量过大")
		}
		if file.Name != "word/document.xml" {
			continue
		}
		if file.UncompressedSize64 > novelBibleTextMaxBytes*4 {
			return "", errors.New("DOCX 解压内容过大")
		}
		r, openErr := file.Open()
		if openErr != nil {
			return "", openErr
		}
		defer r.Close()
		decoder := xml.NewDecoder(io.LimitReader(r, novelBibleTextMaxBytes*4+1))
		var out strings.Builder
		for {
			token, tokenErr := decoder.Token()
			if tokenErr == io.EOF {
				break
			}
			if tokenErr != nil {
				return "", fmt.Errorf("解析 DOCX: %w", tokenErr)
			}
			switch value := token.(type) {
			case xml.CharData:
				if out.Len()+len(value) > novelBibleTextMaxBytes {
					return "", errors.New("DOCX 文本超过 2 MB")
				}
				out.Write(value)
			case xml.EndElement:
				if value.Name.Local == "p" {
					out.WriteByte('\n')
				}
			}
		}
		return strings.TrimSpace(out.String()), nil
	}
	return "", errors.New("DOCX 缺少正文内容")
}

func saveNovelBibleSource(projectID, ext, text string) error {
	dir := filepath.Join(novelsDir(), projectID, "bible")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if ext == "" {
		ext = ".txt"
	}
	return writeFileAtomic(filepath.Join(dir, "source"+ext), []byte(text), 0o600)
}

func novelMemoryPath(projectID string) string {
	return filepath.Join(novelsDir(), projectID, "memory", "memory.json")
}

func loadNovelMemory(projectID string) (NovelLongMemory, error) {
	if !validNovelID(projectID) {
		return NovelLongMemory{}, errors.New("无效的小说项目 ID")
	}
	data, err := os.ReadFile(novelMemoryPath(projectID))
	if errors.Is(err, os.ErrNotExist) {
		return NovelLongMemory{Timeline: []string{}, CharacterStates: map[string]string{}, Facts: []string{}, OpenThreads: []string{}, Constraints: []string{}, Chapters: []NovelChapterMemory{}}, nil
	}
	if err != nil {
		return NovelLongMemory{}, err
	}
	if len(data) > novelMemoryMaxBytes {
		return NovelLongMemory{}, errors.New("小说长记忆文件超过 2 MB")
	}
	var memory NovelLongMemory
	if err := json.Unmarshal(data, &memory); err != nil {
		return NovelLongMemory{}, err
	}
	if memory.CharacterStates == nil {
		memory.CharacterStates = map[string]string{}
	}
	return memory, nil
}

func saveNovelMemory(projectID string, memory NovelLongMemory) error {
	if !validNovelID(projectID) {
		return errors.New("无效的小说项目 ID")
	}
	data, err := json.MarshalIndent(memory, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > novelMemoryMaxBytes {
		return errors.New("小说长记忆超过 2 MB 限制")
	}
	dir := filepath.Dir(novelMemoryPath(projectID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(novelMemoryPath(projectID), data, 0o600)
}

func novelJobPath(projectID, jobID string) string {
	return filepath.Join(novelsDir(), projectID, "jobs", jobID+".json")
}
func saveNovelJob(job NovelAutoWriteJob) error {
	if !validNovelID(job.ProjectID) || !validNovelID(job.ID) {
		return errors.New("无效的自动写作任务 ID")
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(novelJobPath(job.ProjectID, job.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(novelJobPath(job.ProjectID, job.ID), data, 0o600)
}

func loadNovelJob(path string) (NovelAutoWriteJob, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return NovelAutoWriteJob{}, err
	}
	var job NovelAutoWriteJob
	if err := json.Unmarshal(data, &job); err != nil {
		return NovelAutoWriteJob{}, err
	}
	return job, nil
}

func listNovelJobs(projectID string) ([]NovelAutoWriteJob, error) {
	dir := filepath.Join(novelsDir(), projectID, "jobs")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []NovelAutoWriteJob{}, nil
	}
	if err != nil {
		return nil, err
	}
	jobs := make([]NovelAutoWriteJob, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		job, loadErr := loadNovelJob(filepath.Join(dir, entry.Name()))
		if loadErr == nil && job.ProjectID == projectID {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt > jobs[j].CreatedAt })
	return jobs, nil
}

func findNovelJob(jobID string) (NovelAutoWriteJob, error) {
	if !validNovelID(jobID) {
		return NovelAutoWriteJob{}, errors.New("无效的自动写作任务 ID")
	}
	projects, err := os.ReadDir(novelsDir())
	if err != nil {
		return NovelAutoWriteJob{}, err
	}
	for _, project := range projects {
		if !project.IsDir() || !validNovelID(project.Name()) {
			continue
		}
		job, loadErr := loadNovelJob(novelJobPath(project.Name(), jobID))
		if loadErr == nil {
			return job, nil
		}
	}
	return NovelAutoWriteJob{}, errors.New("自动写作任务不存在")
}

func unmarshalNovelJSON(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	if start := strings.Index(raw, "{"); start >= 0 {
		raw = raw[start:]
	}
	if end := strings.LastIndex(raw, "}"); end >= 0 {
		raw = raw[:end+1]
	}
	return json.Unmarshal([]byte(raw), target)
}

func limitNovelStrings(values []string, maxItems, maxRunes int) []string {
	if len(values) > maxItems {
		values = values[len(values)-maxItems:]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := trimRunes(value, maxRunes); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func limitNovelMap(values map[string]string, maxItems, maxRunes int) map[string]string {
	result := make(map[string]string)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(result) >= maxItems {
			break
		}
		value := values[key]
		key = trimRunes(key, 120)
		if key != "" {
			result[key] = trimRunes(value, maxRunes)
		}
	}
	return result
}

func compactNovelMemoryForPrompt(memory NovelLongMemory) NovelLongMemory {
	memory.Bible.Summary = trimRunes(memory.Bible.Summary, 5000)
	memory.Bible.Style = trimRunes(memory.Bible.Style, 2500)
	memory.GlobalSummary = trimRunes(memory.GlobalSummary, 10000)
	memory.Timeline = tailNovelStrings(memory.Timeline, 40, 400)
	memory.CharacterStates = limitNovelMap(memory.CharacterStates, 80, 800)
	memory.Facts = tailNovelStrings(memory.Facts, 80, 400)
	memory.OpenThreads = tailNovelStrings(memory.OpenThreads, 80, 500)
	// Hard constraints are never discarded by recency; the importer already
	// bounds them, and violating one is more costly than losing an old fact.
	memory.Constraints = limitNovelStrings(memory.Constraints, 200, 500)
	if len(memory.Chapters) > 24 {
		memory.Chapters = memory.Chapters[len(memory.Chapters)-24:]
	}
	return memory
}

func tailNovelStrings(values []string, maxItems, maxRunes int) []string {
	if len(values) > maxItems {
		values = values[len(values)-maxItems:]
	}
	return limitNovelStrings(values, maxItems, maxRunes)
}
