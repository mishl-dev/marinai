package bot

import (
	"context"
	"fmt"
	"log"
	"marinai/pkg/skills"
	"marinai/pkg/tools"
)

var skillRegistry *skills.SkillRegistry

func InitSkills(skillsDir string, toolRegistry *tools.Registry) error {
	skillRegistry = skills.NewSkillRegistry(skillsDir, toolRegistry)

	if err := skillRegistry.LoadAll(context.Background()); err != nil {
		return fmt.Errorf("failed to load skills: %w", err)
	}

	manifests := skillRegistry.List()
	log.Printf("Loaded %d skills", len(manifests))
	for _, m := range manifests {
		log.Printf("  - %s: %s", m.Name, m.Description)
	}

	return nil
}

type SkillTool struct {
	registry *skills.SkillRegistry
}

func NewSkillTool() *SkillTool {
	return &SkillTool{registry: skillRegistry}
}

func (t *SkillTool) Name() string {
	return "skill"
}

func (t *SkillTool) Description() string {
	manifests := t.listSkills()
	if len(manifests) == 0 {
		return "Load a specialized skill that provides domain-specific instructions. No skills are currently available."
	}

	desc := `Load a specialized skill that provides domain-specific instructions and workflows.

When you recognize that a task matches one of the available skills, use this tool to load the full skill instructions.

Available skills:
`
	for _, m := range manifests {
		emoji := m.Emoji
		if emoji == "" {
			emoji = "📋"
		}
		desc += fmt.Sprintf("\n  %s %s: %s", emoji, m.Name, m.Description)
	}

	return desc
}

func (t *SkillTool) listSkills() []*skills.SkillManifest {
	if t.registry == nil {
		return nil
	}
	return t.registry.List()
}

func (t *SkillTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type: "object",
		Properties: map[string]tools.PropertySchema{
			"name": {
				Type:        "string",
				Description: "The name of the skill to load",
			},
		},
		Required: []string{"name"},
	}
}

func (t *SkillTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	if t.registry == nil {
		return tools.Result{Success: false, Error: "skill system not initialized"}, nil
	}

	name, _ := params["name"].(string)
	if name == "" {
		return tools.Result{Success: false, Error: "skill name required"}, nil
	}

	skill, ok := t.registry.Get(name)
	if !ok {
		available := t.listSkills()
		names := make([]string, len(available))
		for i, m := range available {
			names[i] = m.Name
		}
		return tools.Result{
			Success: false,
			Error:   fmt.Sprintf("skill '%s' not found. Available: %v", name, names),
		}, nil
	}

	manifest := skill.Manifest()

	result := map[string]any{
		"name":        manifest.Name,
		"description": manifest.Description,
		"version":     manifest.Version,
		"author":      manifest.Author,
	}

	if len(manifest.Prompts) > 0 {
		result["prompts"] = manifest.Prompts
	}

	if len(manifest.Scripts) > 0 {
		result["scripts"] = manifest.Scripts
	}

	if len(manifest.Tools) > 0 {
		toolInfos := make([]map[string]any, len(manifest.Tools))
		for i, tool := range manifest.Tools {
			toolInfos[i] = map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
			}
		}
		result["tools"] = toolInfos
	}

	if len(manifest.Metadata) > 0 {
		result["metadata"] = manifest.Metadata
	}

	return tools.Result{Success: true, Data: result}, nil
}

type ListSkillsTool struct {
	registry *skills.SkillRegistry
}

func NewListSkillsTool() *ListSkillsTool {
	return &ListSkillsTool{registry: skillRegistry}
}

func (t *ListSkillsTool) Name() string {
	return "list_skills"
}

func (t *ListSkillsTool) Description() string {
	return "List all available skills"
}

func (t *ListSkillsTool) Parameters() tools.ParameterSchema {
	return tools.ParameterSchema{
		Type:       "object",
		Properties: map[string]tools.PropertySchema{},
	}
}

func (t *ListSkillsTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	if t.registry == nil {
		return tools.Result{Success: false, Error: "skill system not initialized"}, nil
	}

	manifests := t.registry.List()
	if len(manifests) == 0 {
		return tools.Result{Success: true, Data: "No skills available"}, nil
	}

	var lines []string
	lines = append(lines, "Available skills:")
	for _, m := range manifests {
		emoji := m.Emoji
		if emoji == "" {
			emoji = "📋"
		}
		line := fmt.Sprintf("%s **%s**: %s", emoji, m.Name, m.Description)
		if m.Version != "" {
			line += fmt.Sprintf(" (v%s)", m.Version)
		}
		lines = append(lines, line)
	}

	return tools.Result{Success: true, Data: lines}, nil
}

func RegisterSkillTools(registry *tools.Registry) {
	if skillRegistry == nil {
		return
	}
	registry.Register(NewSkillTool())
	registry.Register(NewListSkillsTool())
	log.Println("Skill tools registered: skill, list_skills")
}
