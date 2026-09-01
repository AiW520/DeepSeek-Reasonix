package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
)

const (
	tuziCreationImageEndpoint = "https://api.tu-zi.com/coding/images/generations"
	tuziCreationImageModel    = "gpt-image-2"
	tuziCreationCredential    = "TUZI_CODING_API_KEY"
	creationImageMaxBytes     = 20 * 1024 * 1024
	creationImageResponseMax  = 80 * 1024 * 1024
)

type CreationImageRequest struct {
	Prompt       string `json:"prompt"`
	Size         string `json:"size"`
	Quality      string `json:"quality"`
	OutputFormat string `json:"outputFormat"`
	Background   string `json:"background"`
}

type CreationImageStatusView struct {
	Configured bool   `json:"configured"`
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint"`
}

type CreationImageView struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Filename   string `json:"filename"`
	Prompt     string `json:"prompt,omitempty"`
	Size       string `json:"size,omitempty"`
	Quality    string `json:"quality,omitempty"`
	Background string `json:"background,omitempty"`
	Mime       string `json:"mime"`
	Bytes      int64  `json:"bytes"`
	CreatedAt  int64  `json:"createdAt"`
	Preview    string `json:"preview,omitempty"`
}

type creationImageMetadata struct {
	ID         string `json:"id"`
	Prompt     string `json:"prompt"`
	Size       string `json:"size"`
	Quality    string `json:"quality"`
	Background string `json:"background"`
	Mime       string `json:"mime"`
	CreatedAt  int64  `json:"createdAt"`
}

type PresentationDraftRequest struct {
	Topic      string `json:"topic"`
	Audience   string `json:"audience"`
	Tone       string `json:"tone"`
	SlideCount int    `json:"slideCount"`
}

type PresentationSlide struct {
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle,omitempty"`
	Bullets      []string `json:"bullets,omitempty"`
	SpeakerNotes string   `json:"speakerNotes,omitempty"`
	Layout       string   `json:"layout,omitempty"`
}

type PresentationOutline struct {
	Title    string              `json:"title"`
	Subtitle string              `json:"subtitle,omitempty"`
	Theme    string              `json:"theme,omitempty"`
	Slides   []PresentationSlide `json:"slides"`
}

func (a *App) CreationImageStatus() CreationImageStatusView {
	credential := config.ResolveCredentialForRootGlobalFirst(a.activeWorkspaceRoot(), tuziCreationCredential)
	return CreationImageStatusView{Configured: credential.Set && credential.Value != "", Model: tuziCreationImageModel, Endpoint: tuziCreationImageEndpoint}
}

func (a *App) GenerateCreationImage(input CreationImageRequest) (CreationImageView, error) {
	credential := config.ResolveCredentialForRootGlobalFirst(a.activeWorkspaceRoot(), tuziCreationCredential)
	if !credential.Set || credential.Value == "" {
		return CreationImageView{}, errors.New("Tuzi Coding 凭据尚未配置，请先在模型设置中保存可用的 Coding API Key")
	}
	cfg, err := config.LoadForRoot(a.activeWorkspaceRoot())
	if err != nil {
		return CreationImageView{}, fmt.Errorf("读取网络设置: %w", err)
	}
	client, err := netclient.NewHTTPClient(cfg.NetworkProxySpec(), netclient.TransportOptions{
		DialTimeout: 20 * time.Second, KeepAlive: 30 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 180 * time.Second,
	})
	if err != nil {
		return CreationImageView{}, fmt.Errorf("创建图片请求客户端: %w", err)
	}
	client.Timeout = 190 * time.Second
	ctx, cancel := context.WithTimeout(a.reqCtx(), 185*time.Second)
	defer cancel()
	return generateCreationImage(ctx, client, tuziCreationImageEndpoint, credential.Value, creationImagesDir(), input)
}

func (a *App) RecentCreationImages(limit int) ([]CreationImageView, error) {
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	dir := creationImagesDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []CreationImageView{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取图片作品库: %w", err)
	}
	views := make([]CreationImageView, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isCreationImageExtension(filepath.Ext(entry.Name())) {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil || info.Size() <= 0 || info.Size() > creationImageMaxBytes {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		view := CreationImageView{
			ID: strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), Path: path,
			Filename: entry.Name(), Mime: creationImageMime(filepath.Ext(entry.Name())), Bytes: info.Size(), CreatedAt: info.ModTime().UnixMilli(),
		}
		if data, readErr := os.ReadFile(path + ".json"); readErr == nil {
			var meta creationImageMetadata
			if json.Unmarshal(data, &meta) == nil {
				view.Prompt, view.Size, view.Quality, view.Background = meta.Prompt, meta.Size, meta.Quality, meta.Background
				if meta.CreatedAt > 0 {
					view.CreatedAt = meta.CreatedAt
				}
			}
		}
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool { return views[i].CreatedAt > views[j].CreatedAt })
	if len(views) > limit {
		views = views[:limit]
	}
	return views, nil
}

func (a *App) CreationImagePreview(id string) (string, error) {
	path, mime, err := resolveCreationImage(id)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取图片: %w", err)
	}
	if len(data) == 0 || len(data) > creationImageMaxBytes {
		return "", errors.New("图片为空或超过预览大小限制")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func (a *App) DraftPresentation(input PresentationDraftRequest) (PresentationOutline, error) {
	input.Topic = strings.TrimSpace(input.Topic)
	input.Audience = strings.TrimSpace(input.Audience)
	input.Tone = strings.TrimSpace(input.Tone)
	if input.Topic == "" {
		return PresentationOutline{}, errors.New("请输入 PPT 主题")
	}
	if len([]rune(input.Topic)) > 300 || len([]rune(input.Audience)) > 200 || len([]rune(input.Tone)) > 100 {
		return PresentationOutline{}, errors.New("PPT 需求过长，请精简后重试")
	}
	if input.SlideCount < 4 {
		input.SlideCount = 6
	}
	if input.SlideCount > 20 {
		input.SlideCount = 20
	}

	root := a.activeWorkspaceRoot()
	cfg, err := config.LoadForRoot(root)
	if err != nil {
		return PresentationOutline{}, fmt.Errorf("读取模型配置: %w", err)
	}
	resolved, _, ok := cfg.ResolveModelWithFallback(cfg.DefaultModel)
	if !ok {
		return PresentationOutline{}, errors.New("当前没有可用的默认模型")
	}
	entry, ok := cfg.ResolveModel(resolved)
	if !ok {
		return PresentationOutline{}, errors.New("无法解析默认模型")
	}
	prov, err := boot.NewProviderWithProxy(entry, cfg.NetworkProxySpec())
	if err != nil {
		return PresentationOutline{}, fmt.Errorf("初始化 PPT 内容模型: %w", err)
	}
	prompt := fmt.Sprintf(`请为桌面应用生成一份可直接制作成 PPT 的中文结构化大纲。
主题：%s
受众：%s
语气：%s
总页数：%d（必须包含封面页）

只输出 JSON，不要 Markdown。格式：
{"title":"主标题","subtitle":"副标题","theme":"graphite","slides":[{"title":"页面标题","subtitle":"可选副标题","bullets":["要点1","要点2"],"speakerNotes":"演讲提示","layout":"cover|section|content|summary"}]}
要求：每页最多 5 个要点；每个要点不超过 36 个汉字；第一页 layout=cover；最后一页 layout=summary；内容要具体，不写空泛套话。`, input.Topic, fallbackText(input.Audience, "通用专业受众"), fallbackText(input.Tone, "专业、清晰、简洁"), input.SlideCount)
	ctx, cancel := context.WithTimeout(a.reqCtx(), 120*time.Second)
	defer cancel()
	stream, err := prov.Stream(ctx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "你是资深演示文稿架构师。严格按用户给定 JSON schema 输出，不要添加代码围栏或解释。"},
			{Role: provider.RoleUser, Content: prompt},
		},
		MaxTokens: 5000,
	})
	if err != nil {
		return PresentationOutline{}, fmt.Errorf("请求 PPT 内容模型: %w", err)
	}
	var output strings.Builder
	for {
		select {
		case <-ctx.Done():
			return PresentationOutline{}, fmt.Errorf("生成 PPT 大纲超时: %w", ctx.Err())
		case chunk, open := <-stream:
			if !open {
				return parsePresentationOutline(output.String(), input.SlideCount)
			}
			switch chunk.Type {
			case provider.ChunkText:
				if output.Len()+len(chunk.Text) > 256*1024 {
					return PresentationOutline{}, errors.New("PPT 大纲响应超过大小限制")
				}
				output.WriteString(chunk.Text)
			case provider.ChunkError:
				return PresentationOutline{}, fmt.Errorf("生成 PPT 大纲: %w", chunk.Err)
			case provider.ChunkDone:
				return parsePresentationOutline(output.String(), input.SlideCount)
			}
		}
	}
}

func generateCreationImage(ctx context.Context, client *http.Client, endpoint, key, outputDir string, input CreationImageRequest) (CreationImageView, error) {
	input, err := validateCreationImageRequest(input)
	if err != nil {
		return CreationImageView{}, err
	}
	payload := map[string]any{
		"model": tuziCreationImageModel, "prompt": input.Prompt, "n": 1,
		"size": input.Size, "quality": input.Quality, "output_format": input.OutputFormat, "background": input.Background,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return CreationImageView{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return CreationImageView{}, fmt.Errorf("创建图片请求: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Reasonix-Desktop/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return CreationImageView{}, errors.New("图片生成失败或超时；为避免重复计费，本次没有自动重试")
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, creationImageResponseMax+1))
	if err != nil || len(responseBody) > creationImageResponseMax {
		return CreationImageView{}, errors.New("图片生成服务返回了无效或过大的响应")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CreationImageView{}, fmt.Errorf("图片生成服务返回 HTTP %d：%s", resp.StatusCode, safeCreationProviderMessage(responseBody, key))
	}
	var result struct {
		Data []json.RawMessage `json:"data"`
	}
	if json.Unmarshal(responseBody, &result) != nil || len(result.Data) == 0 {
		return CreationImageView{}, errors.New("图片生成服务没有返回图片数据")
	}
	imageBytes, mime, err := decodeCreationImageItem(ctx, client, result.Data[0])
	if err != nil {
		return CreationImageView{}, err
	}
	if len(imageBytes) == 0 || len(imageBytes) > creationImageMaxBytes {
		return CreationImageView{}, errors.New("生成图片为空或超过 20MB 限制")
	}
	if validated, detected := safeRemoteMarkdownImage(imageBytes); detected == "" || detected != mime || len(validated) != len(imageBytes) {
		return CreationImageView{}, errors.New("生成结果不是受支持的 PNG、JPEG 或 WebP 图片")
	}
	if err := validateMarkdownImageBytes(imageBytes, mime); err != nil {
		return CreationImageView{}, errors.New("生成图片无法安全解码或像素尺寸过大")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return CreationImageView{}, fmt.Errorf("创建图片作品目录: %w", err)
	}
	id := newCreationImageID()
	ext := extensionForCreationMime(mime)
	path := filepath.Join(outputDir, id+ext)
	if err := writeCreationFile(path, imageBytes); err != nil {
		return CreationImageView{}, err
	}
	createdAt := time.Now().UnixMilli()
	meta := creationImageMetadata{ID: id, Prompt: input.Prompt, Size: input.Size, Quality: input.Quality, Background: input.Background, Mime: mime, CreatedAt: createdAt}
	if encoded, marshalErr := json.Marshal(meta); marshalErr == nil {
		_ = writeCreationFile(path+".json", encoded)
	}
	return CreationImageView{
		ID: id, Path: path, Filename: filepath.Base(path), Prompt: input.Prompt, Size: input.Size,
		Quality: input.Quality, Background: input.Background, Mime: mime, Bytes: int64(len(imageBytes)), CreatedAt: createdAt,
		Preview: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(imageBytes),
	}, nil
}

func validateCreationImageRequest(input CreationImageRequest) (CreationImageRequest, error) {
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Prompt == "" {
		return input, errors.New("请输入图片描述")
	}
	if len(input.Prompt) > 64*1024 || !utf8.ValidString(input.Prompt) {
		return input, errors.New("图片描述无效或超过 64KB")
	}
	input.Size = strings.ToLower(strings.TrimSpace(input.Size))
	input.Quality = strings.ToLower(strings.TrimSpace(input.Quality))
	input.OutputFormat = strings.ToLower(strings.TrimSpace(input.OutputFormat))
	input.Background = strings.ToLower(strings.TrimSpace(input.Background))
	if input.Size == "" {
		input.Size = "auto"
	}
	if input.Quality == "" {
		input.Quality = "auto"
	}
	if input.OutputFormat == "" {
		input.OutputFormat = "png"
	}
	if input.Background == "" {
		input.Background = "auto"
	}
	if !oneOf(input.Size, "auto", "1024x1024", "1536x1024", "1024x1536") {
		return input, errors.New("图片尺寸不受支持")
	}
	if !oneOf(input.Quality, "auto", "low", "medium", "high") {
		return input, errors.New("图片质量不受支持")
	}
	if !oneOf(input.OutputFormat, "png", "jpeg", "webp") {
		return input, errors.New("图片格式不受支持")
	}
	if !oneOf(input.Background, "auto", "transparent", "opaque") {
		return input, errors.New("背景模式不受支持")
	}
	if input.OutputFormat == "jpeg" && input.Background == "transparent" {
		return input, errors.New("JPEG 不支持透明背景")
	}
	return input, nil
}

func decodeCreationImageItem(ctx context.Context, client *http.Client, raw json.RawMessage) ([]byte, string, error) {
	var direct string
	if json.Unmarshal(raw, &direct) == nil && direct != "" {
		return downloadCreationImage(ctx, client, direct)
	}
	var item struct {
		URL     string `json:"url"`
		OSSURL  string `json:"oss_url"`
		Base64  string `json:"base64"`
		B64JSON string `json:"b64_json"`
	}
	if json.Unmarshal(raw, &item) != nil {
		return nil, "", errors.New("图片生成服务返回了不支持的数据格式")
	}
	encoded := item.B64JSON
	if encoded == "" {
		encoded = item.Base64
	}
	if encoded != "" {
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(data) > creationImageMaxBytes {
			return nil, "", errors.New("图片生成服务返回了无效的 Base64 图片")
		}
		_, mime := safeRemoteMarkdownImage(data)
		if mime == "" || !oneOf(mime, "image/png", "image/jpeg", "image/webp") {
			return nil, "", errors.New("图片生成服务返回了不支持的图片类型")
		}
		return data, mime, nil
	}
	imageURL := item.URL
	if imageURL == "" {
		imageURL = item.OSSURL
	}
	if imageURL == "" {
		return nil, "", errors.New("图片生成服务没有返回可下载的图片")
	}
	return downloadCreationImage(ctx, client, imageURL)
}

func downloadCreationImage(ctx context.Context, base *http.Client, rawURL string) ([]byte, string, error) {
	validated, err := validateRemoteMarkdownImageURL(rawURL)
	if err != nil {
		return nil, "", errors.New("图片下载地址未通过安全检查")
	}
	client := base
	if guarded, guardErr := newRemoteMarkdownImageClient(netclient.ProxySpec{Mode: netclient.ModeAuto}); guardErr == nil {
		client = guarded
	}
	copyClient := *client
	copyClient.Timeout = 60 * time.Second
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("图片下载重定向过多")
		}
		_, err := validateRemoteMarkdownImageURL(req.URL.String())
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validated, nil)
	if err != nil {
		return nil, "", errors.New("无法创建图片下载请求")
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp")
	req.Header.Set("User-Agent", "Reasonix-Desktop/1.0")
	resp, err := copyClient.Do(req)
	if err != nil {
		return nil, "", errors.New("生成成功，但下载图片失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("生成成功，但图片下载返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, creationImageMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > creationImageMaxBytes {
		return nil, "", errors.New("下载的图片为空或超过大小限制")
	}
	_, mime := safeRemoteMarkdownImage(data)
	if !oneOf(mime, "image/png", "image/jpeg", "image/webp") {
		return nil, "", errors.New("下载内容不是受支持的图片")
	}
	return data, mime, nil
}

func parsePresentationOutline(raw string, expectedSlides int) (PresentationOutline, error) {
	raw = strings.TrimSpace(raw)
	if start := strings.Index(raw, "{"); start >= 0 {
		if end := strings.LastIndex(raw, "}"); end >= start {
			raw = raw[start : end+1]
		}
	}
	var outline PresentationOutline
	if json.Unmarshal([]byte(raw), &outline) != nil {
		return PresentationOutline{}, errors.New("模型返回的 PPT 大纲格式无效，请重试")
	}
	outline.Title = boundedText(outline.Title, 120)
	outline.Subtitle = boundedText(outline.Subtitle, 180)
	outline.Theme = strings.ToLower(strings.TrimSpace(outline.Theme))
	if outline.Theme == "" {
		outline.Theme = "graphite"
	}
	if outline.Title == "" || len(outline.Slides) < 2 {
		return PresentationOutline{}, errors.New("模型返回的 PPT 大纲内容不完整")
	}
	if len(outline.Slides) > min(20, expectedSlides+3) {
		outline.Slides = outline.Slides[:min(20, expectedSlides+3)]
	}
	for i := range outline.Slides {
		slide := &outline.Slides[i]
		slide.Title = boundedText(slide.Title, 120)
		slide.Subtitle = boundedText(slide.Subtitle, 180)
		slide.SpeakerNotes = boundedText(slide.SpeakerNotes, 800)
		slide.Layout = strings.ToLower(strings.TrimSpace(slide.Layout))
		if !oneOf(slide.Layout, "cover", "section", "content", "summary") {
			slide.Layout = "content"
		}
		if len(slide.Bullets) > 6 {
			slide.Bullets = slide.Bullets[:6]
		}
		for j := range slide.Bullets {
			slide.Bullets[j] = boundedText(slide.Bullets[j], 160)
		}
	}
	outline.Slides[0].Layout = "cover"
	return outline, nil
}

func creationImagesDir() string {
	return filepath.Join(config.ReasonixHomeDir(), "creations", "images")
}

func resolveCreationImage(id string) (string, string, error) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 80 || strings.ContainsAny(id, `/\\.:`) {
		return "", "", errors.New("图片标识无效")
	}
	for _, ext := range []string{".png", ".jpg", ".webp"} {
		path := filepath.Join(creationImagesDir(), id+ext)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, creationImageMime(ext), nil
		}
	}
	return "", "", errors.New("图片不存在")
}

func writeCreationFile(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".reasonix-creation-*")
	if err != nil {
		return fmt.Errorf("创建临时作品文件: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("写入作品文件: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("保存作品文件: %w", err)
	}
	return nil
}

func newCreationImageID() string {
	var random [6]byte
	_, _ = rand.Read(random[:])
	return fmt.Sprintf("image-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(random[:]))
}

func safeCreationProviderMessage(body []byte, key string) string {
	message := "请求失败"
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		if value, ok := payload["message"].(string); ok && value != "" {
			message = value
		}
		if value, ok := payload["error"].(map[string]any); ok {
			if text, ok := value["message"].(string); ok && text != "" {
				message = text
			}
		}
	}
	message = strings.ReplaceAll(message, key, "[REDACTED]")
	message = strings.ReplaceAll(strings.ReplaceAll(message, "\r", " "), "\n", " ")
	return boundedText(message, 500)
}

func oneOf(value string, values ...string) bool {
	return slices.Contains(values, value)
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func boundedText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func isCreationImageExtension(ext string) bool {
	return oneOf(strings.ToLower(ext), ".png", ".jpg", ".webp")
}

func creationImageMime(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

func extensionForCreationMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
