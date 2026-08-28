package main

import (
	"strings"
	"testing"
)

func TestMarketplaceCatalogIsPinnedAndReviewed(t *testing.T) {
	view := (&App{}).MarketplaceCatalog("all", "")
	if len(view.Entries) < 5 {
		t.Fatalf("catalog has %d entries; want launch set", len(view.Entries))
	}
	seen := map[string]bool{}
	for _, entry := range view.Entries {
		if seen[entry.ID] {
			t.Fatalf("duplicate marketplace id %q", entry.ID)
		}
		seen[entry.ID] = true
		if !strings.HasPrefix(entry.Repository, "https://github.com/") {
			t.Errorf("%s has unreviewed host: %s", entry.ID, entry.Repository)
		}
		if strings.Contains(entry.Repository, "/tree/") {
			t.Errorf("%s repository contains a revision path: %s", entry.ID, entry.Repository)
		}
		if len(entry.Commit) != 40 {
			t.Errorf("%s commit is not a full SHA: %q", entry.ID, entry.Commit)
		}
		for _, r := range entry.Commit {
			if !strings.ContainsRune("0123456789abcdef", r) {
				t.Errorf("%s has invalid commit: %q", entry.ID, entry.Commit)
				break
			}
		}
		if entry.License == "" || entry.Author == "" {
			t.Errorf("%s lacks provenance", entry.ID)
		}
	}
}

func TestMarketplaceInstallSourcePinsSkillExactlyOnce(t *testing.T) {
	entry := MarketplaceEntry{
		Kind:       "skill",
		Repository: "https://github.com/runesleo/claude-video-kit/tree/old-revision",
		Commit:     "f09790c6e90e610b9dbdec0d1983bd5abeecd0bf",
	}
	want := "https://github.com/runesleo/claude-video-kit/tree/" + entry.Commit
	if got := marketplaceInstallSource(entry); got != want {
		t.Fatalf("marketplaceInstallSource() = %q, want %q", got, want)
	}
}

func TestMarketplaceRejectsArbitraryID(t *testing.T) {
	if _, err := marketplaceEntry("https://github.com/evil/unreviewed"); err == nil {
		t.Fatal("arbitrary source was accepted as a marketplace id")
	}
}

func TestMarketplaceFiltersKindAndQuery(t *testing.T) {
	a := &App{}
	for _, entry := range a.MarketplaceCatalog("skill", "video").Entries {
		if entry.Kind != "skill" {
			t.Fatalf("plugin leaked into skill catalog: %+v", entry)
		}
	}
	if got := a.MarketplaceCatalog("plugin", "superpowers"); len(got.Entries) != 1 || got.Entries[0].ID != "plugin-superpowers" {
		t.Fatalf("query result = %+v", got.Entries)
	}
}
