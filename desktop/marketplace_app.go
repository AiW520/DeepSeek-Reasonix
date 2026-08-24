package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"reasonix/internal/installsource"
)

// MarketplaceEntry is a reviewed, immutable catalog item. The frontend only
// sends the id back; source and commit are resolved again on the host.
type MarketplaceEntry struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"` // plugin | skill
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Repository   string   `json:"repository"`
	Commit       string   `json:"commit"`
	License      string   `json:"license"`
	Author       string   `json:"author"`
	Capabilities []string `json:"capabilities,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Risk         string   `json:"risk"`
	RiskReasons  []string `json:"riskReasons,omitempty"`
}

type MarketplaceCatalogView struct {
	Entries []MarketplaceEntry `json:"entries"`
	Cached  bool               `json:"cached"`
	Warning string             `json:"warning,omitempty"`
}

var reviewedMarketplace = []MarketplaceEntry{
	{ID: "plugin-superpowers", Kind: "plugin", Name: "Superpowers", Description: "Battle-tested software development workflow with planning, TDD, debugging, and review skills.", Category: "programming", Repository: "https://github.com/obra/superpowers", Commit: "b36e0829c6d0140e93cfef2ca599b1b07d4a7797", License: "MIT", Author: "obra", Capabilities: []string{"skills", "commands"}, Risk: "medium", RiskReasons: []string{"Installs a multi-skill plugin package"}},
	{ID: "plugin-agents", Kind: "plugin", Name: "Agents", Description: "Curated specialist agents and developer workflow plugins for daily engineering work.", Category: "work", Repository: "https://github.com/wshobson/agents", Commit: "367cb6a4a182cf7e9b0a17c9429f7411ddd9cf35", License: "MIT", Author: "wshobson", Capabilities: []string{"skills", "agents", "commands"}, Risk: "medium", RiskReasons: []string{"May add agents and command handlers"}},
	{ID: "plugin-claude-community", Kind: "plugin", Name: "Claude Plugins Community", Description: "Community-maintained plugin collection with reusable productivity capabilities.", Category: "work", Repository: "https://github.com/anthropics/claude-plugins-community", Commit: "24a5ecd5dd88e201e185e1174b7797a4e857dd67", License: "Apache-2.0", Author: "Anthropic community", Capabilities: []string{"plugins", "skills"}, Risk: "medium", RiskReasons: []string{"Community package; review capabilities before approval"}},
	{ID: "skill-video-shotcraft", Kind: "skill", Name: "Video Shotcraft", Description: "Plan and execute polished video shot lists, coverage, and production workflows.", Category: "video", Repository: "https://github.com/Vincentwei1021/video-shotcraft/tree/0d6f0b57f0d4d6700761644c07f7ef03c3e50234", Commit: "0d6f0b57f0d4d6700761644c07f7ef03c3e50234", License: "Apache-2.0", Author: "Vincentwei1021", Capabilities: []string{"skill"}, Risk: "low"},
	{ID: "skill-video-kit", Kind: "skill", Name: "Claude Video Kit", Description: "A structured assistant workflow for scripting, editing and delivering video projects.", Category: "video", Repository: "https://github.com/runesleo/claude-video-kit/tree/f09790c6e90e610b9dbdec0d1983bd5abeecd0bf", Commit: "f09790c6e90e610b9dbdec0d1983bd5abeecd0bf", License: "MIT", Author: "runesleo", Capabilities: []string{"skill"}, Risk: "low"},
	{ID: "skill-remotion-motion", Kind: "skill", Name: "Remotion Motion Graphics", Description: "Motion graphics planning and Remotion production guidance for code-driven video.", Category: "video", Repository: "https://github.com/haidrrrry/claude-remotion-skill/tree/1dcbe5e3fc6cf970bd10d3cc05f0a8a5d19d0383", Commit: "1dcbe5e3fc6cf970bd10d3cc05f0a8a5d19d0383", License: "MIT", Author: "haidrrrry", Capabilities: []string{"skill"}, Risk: "low"},
}

func marketplaceEntry(id string) (MarketplaceEntry, error) {
	for _, entry := range reviewedMarketplace {
		if entry.ID == strings.TrimSpace(id) {
			return entry, nil
		}
	}
	return MarketplaceEntry{}, fmt.Errorf("marketplace item %q is not in the reviewed catalog", id)
}

func (a *App) MarketplaceCatalog(kind, query string) MarketplaceCatalogView {
	kind, query = strings.ToLower(strings.TrimSpace(kind)), strings.ToLower(strings.TrimSpace(query))
	out := MarketplaceCatalogView{Entries: make([]MarketplaceEntry, 0, len(reviewedMarketplace)), Cached: true}
	for _, entry := range reviewedMarketplace {
		if kind != "" && kind != "all" && entry.Kind != kind {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(entry.Name+" "+entry.Description+" "+entry.Category+" "+strings.Join(entry.Capabilities, " ")), query) {
			continue
		}
		out.Entries = append(out.Entries, entry)
	}
	return out
}

func (a *App) PlanMarketplaceInstall(id string) (string, error) {
	entry, err := marketplaceEntry(id)
	if err != nil {
		return "", err
	}
	source := entry.Repository
	if entry.Kind == "skill" { source += "/tree/" + entry.Commit }
	body := map[string]any{"source": source, "kind": entry.Kind, "apply": false, "mode": "copy"}
	raw, _ := json.Marshal(body)
	out, err := installsource.NewTool(installsource.Options{ProjectRoot: a.activeWorkspaceRoot()}).Execute(context.Background(), raw)
	if err != nil {
		return "", err
	}
	if entry.Kind == "plugin" {
		var plan struct {
			Actions []struct {
				Commit string `json:"commit"`
			} `json:"actions"`
		}
		if json.Unmarshal([]byte(out), &plan) != nil || len(plan.Actions) == 0 || !strings.EqualFold(plan.Actions[0].Commit, entry.Commit) {
			return "", fmt.Errorf("reviewed snapshot for %s no longer matches GitHub; catalog update required", entry.Name)
		}
	}
	return out, nil
}

func (a *App) InstallMarketplace(id, planID string) (string, error) {
	entry, err := marketplaceEntry(id)
	if err != nil {
		return "", err
	}
	source := entry.Repository
	if entry.Kind == "skill" { source += "/tree/" + entry.Commit }
	body := map[string]any{"source": source, "kind": entry.Kind, "apply": true, "mode": "copy", "planId": strings.TrimSpace(planID)}
	raw, _ := json.Marshal(body)
	if entry.Kind == "plugin" {
		return a.InstallPlugin(source, PluginInstallOptions{PlanID: strings.TrimSpace(planID)})
	}
	out, err := installsource.NewTool(installsource.Options{ProjectRoot: a.activeWorkspaceRoot()}).Execute(context.Background(), raw)
	if err == nil {
		a.bumpExtensionGeneration()
		a.invalidateSkillRootsCache()
		_ = a.rebuild()
	}
	return out, err
}
