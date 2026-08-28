package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGenerateCreationImageUsesFixedModelAndPersistsImage(t *testing.T) {
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var gotModel, gotPrompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header missing")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		gotModel, _ = payload["model"].(string)
		gotPrompt, _ = payload["prompt"].(string)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(pngBytes)}}})
	}))
	defer server.Close()

	dir := t.TempDir()
	view, err := generateCreationImage(context.Background(), server.Client(), server.URL, "test-key", dir, CreationImageRequest{
		Prompt: "一张安静的工作台", Size: "1024x1024", Quality: "medium", OutputFormat: "png", Background: "opaque",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotModel != tuziCreationImageModel || gotPrompt != "一张安静的工作台" {
		t.Fatalf("request = model %q prompt %q", gotModel, gotPrompt)
	}
	if view.ID == "" || view.Mime != "image/png" || !strings.HasPrefix(view.Preview, "data:image/png;base64,") {
		t.Fatalf("view = %#v", view)
	}
	if info, err := os.Stat(view.Path); err != nil || info.Size() == 0 {
		t.Fatalf("saved image = %v, %v", info, err)
	}
	if _, err := os.Stat(view.Path + ".json"); err != nil {
		t.Fatalf("metadata missing: %v", err)
	}
}

func TestValidateCreationImageRequestRejectsUnsupportedCombinations(t *testing.T) {
	if _, err := validateCreationImageRequest(CreationImageRequest{Prompt: "x", Size: "2048x2048"}); err == nil {
		t.Fatal("unsupported size accepted")
	}
	if _, err := validateCreationImageRequest(CreationImageRequest{Prompt: "x", OutputFormat: "jpeg", Background: "transparent"}); err == nil {
		t.Fatal("transparent JPEG accepted")
	}
	got, err := validateCreationImageRequest(CreationImageRequest{Prompt: " x "})
	if err != nil {
		t.Fatal(err)
	}
	if got.Prompt != "x" || got.Size != "auto" || got.Quality != "auto" || got.OutputFormat != "png" || got.Background != "auto" {
		t.Fatalf("defaults = %#v", got)
	}
}

func TestParsePresentationOutlineBoundsAndNormalizes(t *testing.T) {
	raw := "```json\n" + `{"title":"季度复盘","subtitle":"团队进展","slides":[{"title":"封面","layout":"content"},{"title":"关键进展","layout":"unknown","bullets":["A","B","C","D","E","F","G"]},{"title":"下一步","layout":"summary"}]}` + "\n```"
	outline, err := parsePresentationOutline(raw, 6)
	if err != nil {
		t.Fatal(err)
	}
	if outline.Theme != "graphite" || outline.Slides[0].Layout != "cover" || outline.Slides[1].Layout != "content" {
		t.Fatalf("outline normalization = %#v", outline)
	}
	if len(outline.Slides[1].Bullets) != 6 {
		t.Fatalf("bullets = %d", len(outline.Slides[1].Bullets))
	}
}
