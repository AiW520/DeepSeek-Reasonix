package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNovelProjectPersistenceAndVersions(t *testing.T) {
	root := t.TempDir()
	project := NovelProject{
		ID: "novel-test", Title: "雾港来信", Genre: "悬疑", Premise: "一封来自未来的信。",
		Characters: []NovelCharacter{}, Outlines: []NovelOutline{}, Chapters: []NovelChapter{
			{ID: "chapter-1", Title: "第一章", Content: "旧版本正文", Status: "draft", Order: 1},
		},
	}
	if err := saveNovelProjectAt(root, project, false); err != nil {
		t.Fatalf("save project: %v", err)
	}
	loaded, err := loadNovelProjectAt(root, project.ID)
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	if loaded.Title != project.Title || loaded.Chapters[0].WordCount == 0 {
		t.Fatalf("unexpected loaded project: %+v", loaded)
	}
	if err := saveNovelVersionAt(root, project.ID, loaded.Chapters[0], "测试版本"); err != nil {
		t.Fatalf("save version: %v", err)
	}
	entries, err := listNovelProjectsAt(root)
	if err != nil || len(entries) != 1 || entries[0].WordCount == 0 {
		t.Fatalf("list projects = %+v, %v", entries, err)
	}
}

func TestNovelProjectRejectsUnsafeIDAndOversizedChapter(t *testing.T) {
	root := t.TempDir()
	if err := saveNovelProjectAt(root, NovelProject{ID: "../escape", Title: "bad"}, false); err == nil {
		t.Fatal("expected unsafe id rejection")
	}
	project := NovelProject{ID: "novel-safe", Title: "safe", Chapters: []NovelChapter{{ID: "chapter-1", Title: "large", Content: strings.Repeat("x", novelMaxContentBytes+1)}}}
	if err := saveNovelProjectAt(root, project, false); err == nil {
		t.Fatal("expected oversized chapter rejection")
	}
}

func TestParseNovelAIResult(t *testing.T) {
	result, err := parseNovelAIResult("```json\n{\"content\":\"正文\",\"summary\":\"摘要\"}\n```", "draft")
	if err != nil || result.Content != "正文" {
		t.Fatalf("parse result = %+v, %v", result, err)
	}
	review, err := parseNovelAIResult(`{"summary":"通过","issues":[]}`, "review-final")
	if err != nil || review.Issues == nil {
		t.Fatalf("parse review = %+v, %v", review, err)
	}
}

func TestBuildNovelDOCX(t *testing.T) {
	data, err := buildNovelDOCX(NovelProject{Title: "测试小说", Chapters: []NovelChapter{{Title: "第一章", Content: "第一段\n\n第二段", Order: 1}}})
	if err != nil {
		t.Fatalf("build docx: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("read docx zip: %v", err)
	}
	wanted := map[string]bool{"[Content_Types].xml": false, "_rels/.rels": false, "word/document.xml": false}
	for _, file := range zr.File {
		if _, ok := wanted[file.Name]; ok {
			wanted[file.Name] = true
		}
	}
	for name, found := range wanted {
		if !found {
			t.Fatalf("docx missing %s", name)
		}
	}
}

func TestReadNovelBibleTextAndDOCX(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "bible.md")
	if err := os.WriteFile(textPath, []byte("# 世界观\n雾港允许交易记忆。"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err := readNovelBible(textPath)
	if err != nil || !strings.Contains(text, "交易记忆") {
		t.Fatalf("read markdown bible = %q, %v", text, err)
	}
	docx, err := buildNovelDOCX(NovelProject{Title: "作品圣经", Chapters: []NovelChapter{{Title: "人物", Content: "林澈必须保持克制。", Order: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	docxPath := filepath.Join(dir, "bible.docx")
	if err := os.WriteFile(docxPath, docx, 0o600); err != nil {
		t.Fatal(err)
	}
	text, err = readNovelBible(docxPath)
	if err != nil || !strings.Contains(text, "林澈必须保持克制") {
		t.Fatalf("read docx bible = %q, %v", text, err)
	}
}

func TestNovelMemoryBoundsAndJSONExtraction(t *testing.T) {
	values := []string{"  一  ", "二", "三"}
	got := limitNovelStrings(values, 2, 1)
	if len(got) != 2 || got[0] != "二" || got[1] != "三" {
		t.Fatalf("limitNovelStrings = %#v", got)
	}
	var parsed struct {
		Summary string `json:"summary"`
	}
	if err := unmarshalNovelJSON("```json\n{\"summary\":\"可恢复\"}\n```", &parsed); err != nil || parsed.Summary != "可恢复" {
		t.Fatalf("unmarshalNovelJSON = %+v, %v", parsed, err)
	}
	timeline := make([]string, 50)
	for i := range timeline {
		timeline[i] = "事件"
	}
	memory := compactNovelMemoryForPrompt(NovelLongMemory{Timeline: timeline, Constraints: []string{"主角不能复活"}})
	if len(memory.Timeline) != 40 || len(memory.Constraints) != 1 {
		t.Fatalf("compact memory lost required context: %+v", memory)
	}
}
