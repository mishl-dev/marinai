package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"marinai/pkg/tools"
)

type SkillManifest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Version     string            `json:"version,omitempty"`
	Author      string            `json:"author,omitempty"`
	Homepage    string            `json:"homepage,omitempty"`
	Emoji       string            `json:"emoji,omitempty"`
	OS          []string          `json:"os,omitempty"`
	Requires    SkillRequires     `json:"requires,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Tools       []ToolDefinition  `json:"tools,omitempty"`
	Scripts     map[string]string `json:"scripts,omitempty"`
	Prompts     map[string]string `json:"prompts,omitempty"`
}

type SkillRequires struct {
	Bins  []string `json:"bins,omitempty"`
	Files []string `json:"files,omitempty"`
	Env   []string `json:"env,omitempty"`
}

type ToolDefinition struct {
	Name        string                                                                                             `json:"name"`
	Description string                                                                                             `json:"description"`
	Parameters  map[string]ParamDef                                                                                `json:"parameters"`
	Required    []string                                                                                           `json:"required,omitempty"`
	Execute     string                                                                                             `json:"execute,omitempty"`
	Script      string                                                                                             `json:"script,omitempty"`
	Handler     func(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) `json:"-"`
}

type ParamDef struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

type Skill struct {
	manifest *SkillManifest
	path     string
	loadedAt time.Time
	checksum string
	mu       sync.RWMutex
}

func (s *Skill) Manifest() *SkillManifest {
	return s.manifest
}

func (s *Skill) Path() string {
	return s.path
}

func (s *Skill) LoadedAt() time.Time {
	return s.loadedAt
}

type SkillRegistry struct {
	mu        sync.RWMutex
	skills    map[string]*Skill
	tools     *tools.Registry
	skillsDir string
	watcher   *SkillWatcher
}

func NewSkillRegistry(skillsDir string, toolRegistry *tools.Registry) *SkillRegistry {
	return &SkillRegistry{
		skills:    make(map[string]*Skill),
		tools:     toolRegistry,
		skillsDir: skillsDir,
	}
}

func (sr *SkillRegistry) LoadAll(ctx context.Context) error {
	if sr.skillsDir == "" {
		return nil
	}

	entries, err := os.ReadDir(sr.skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasSuffix(entry.Name(), ".md") && !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		skillPath := filepath.Join(sr.skillsDir, entry.Name())
		if err := sr.Load(ctx, skillPath); err != nil {
			fmt.Printf("[Skills] Failed to load %s: %v\n", entry.Name(), err)
			continue
		}
	}

	return nil
}

func (sr *SkillRegistry) Load(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	var skill *Skill

	if info.IsDir() {
		skill, err = sr.loadFromDir(path)
	} else {
		skill, err = sr.loadFromFile(path)
	}

	if err != nil {
		return err
	}

	sr.mu.Lock()
	sr.skills[skill.manifest.Name] = skill
	sr.mu.Unlock()

	for _, toolDef := range skill.manifest.Tools {
		if err := sr.registerTool(skill, toolDef); err != nil {
			fmt.Printf("[Skills] Failed to register tool %s: %v\n", toolDef.Name, err)
		}
	}

	return nil
}

func (sr *SkillRegistry) loadFromDir(dir string) (*Skill, error) {
	// Try SKILL.md first
	skillMd := filepath.Join(dir, "SKILL.md")
	if _, err := os.Stat(skillMd); err == nil {
		return sr.loadFromSKILLMd(skillMd, dir)
	}

	// Try skill.json
	skillJson := filepath.Join(dir, "skill.json")
	if _, err := os.Stat(skillJson); err == nil {
		return sr.loadFromJson(skillJson, dir)
	}

	return nil, fmt.Errorf("no SKILL.md or skill.json found in %s", dir)
}

func (sr *SkillRegistry) loadFromFile(path string) (*Skill, error) {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)

	switch ext {
	case ".md":
		return sr.loadFromSKILLMd(path, dir)
	case ".json":
		return sr.loadFromJson(path, dir)
	default:
		return nil, fmt.Errorf("unsupported skill file: %s", ext)
	}
}

func (sr *SkillRegistry) loadFromSKILLMd(path string, dir string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	manifest, err := ParseSKILLMd(string(content))
	if err != nil {
		return nil, err
	}

	checksum := sr.computeChecksum(content)

	return &Skill{
		manifest: manifest,
		path:     dir,
		loadedAt: time.Now(),
		checksum: checksum,
	}, nil
}

func (sr *SkillRegistry) loadFromJson(path string, dir string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest SkillManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, err
	}

	checksum := sr.computeChecksum(content)

	return &Skill{
		manifest: &manifest,
		path:     dir,
		loadedAt: time.Now(),
		checksum: checksum,
	}, nil
}

func (sr *SkillRegistry) registerTool(skill *Skill, def ToolDefinition) error {
	tool := &skillTool{
		skill: skill,
		def:   def,
		name:  def.Name,
	}

	if def.Name == "" {
		tool.name = skill.manifest.Name
	}

	if sr.tools != nil {
		return sr.tools.Register(tool)
	}
	return nil
}

func (sr *SkillRegistry) Get(name string) (*Skill, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	skill, ok := sr.skills[name]
	return skill, ok
}

func (sr *SkillRegistry) List() []*SkillManifest {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	manifests := make([]*SkillManifest, 0, len(sr.skills))
	for _, skill := range sr.skills {
		manifests = append(manifests, skill.manifest)
	}
	return manifests
}

func (sr *SkillRegistry) Unload(name string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	delete(sr.skills, name)
	return nil
}

func (sr *SkillRegistry) Reload(name string) error {
	sr.mu.RLock()
	skill, ok := sr.skills[name]
	sr.mu.RUnlock()

	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}

	return sr.Load(context.Background(), skill.path)
}

func (sr *SkillRegistry) computeChecksum(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:8])
}

type skillTool struct {
	skill *Skill
	def   ToolDefinition
	name  string
}

func (t *skillTool) Name() string {
	return t.name
}

func (t *skillTool) Description() string {
	if t.def.Description != "" {
		return t.def.Description
	}
	return t.skill.manifest.Description
}

func (t *skillTool) Parameters() tools.ParameterSchema {
	props := make(map[string]tools.PropertySchema)
	for name, param := range t.def.Parameters {
		props[name] = tools.PropertySchema{
			Type:        param.Type,
			Description: param.Description,
			Enum:        param.Enum,
		}
	}

	return tools.ParameterSchema{
		Type:       "object",
		Properties: props,
		Required:   t.def.Required,
	}
}

func (t *skillTool) Execute(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	if t.def.Handler != nil {
		return t.def.Handler(ctx, params, toolCtx)
	}

	if t.def.Script != "" {
		return t.executeScript(ctx, params, toolCtx)
	}

	return tools.Result{Success: true, Data: map[string]any{
		"skill":   t.skill.manifest.Name,
		"tool":    t.name,
		"params":  params,
		"prompt":  "Skill loaded. Use the skill's prompts and scripts to complete the task.",
		"prompts": t.skill.manifest.Prompts,
		"scripts": t.skill.manifest.Scripts,
	}}, nil
}

func (t *skillTool) executeScript(ctx context.Context, params map[string]any, toolCtx *tools.ToolContext) (tools.Result, error) {
	scriptPath := filepath.Join(t.skill.path, t.def.Script)

	info, err := os.Stat(scriptPath)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("script not found: %s", t.def.Script)}, nil
	}

	_ = info

	return tools.Result{Success: true, Data: map[string]any{
		"message": fmt.Sprintf("Script %s ready to execute", t.def.Script),
		"path":    scriptPath,
		"params":  params,
	}}, nil
}

type SkillWatcher struct {
	registry *SkillRegistry
	stopCh   chan struct{}
}

func NewSkillWatcher(registry *SkillRegistry) *SkillWatcher {
	return &SkillWatcher{
		registry: registry,
		stopCh:   make(chan struct{}),
	}
}

func (w *SkillWatcher) Start() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.checkForChanges()
		case <-w.stopCh:
			return
		}
	}
}

func (w *SkillWatcher) Stop() {
	close(w.stopCh)
}

func (w *SkillWatcher) checkForChanges() {
	w.registry.mu.RLock()
	skills := make([]*Skill, 0, len(w.registry.skills))
	for _, s := range w.registry.skills {
		skills = append(skills, s)
	}
	w.registry.mu.RUnlock()

	for _, skill := range skills {
		manifestPath := filepath.Join(skill.path, "SKILL.md")
		if _, err := os.Stat(manifestPath); err != nil {
			manifestPath = filepath.Join(skill.path, "skill.json")
		}

		content, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		checksum := w.registry.computeChecksum(content)
		if checksum != skill.checksum {
			fmt.Printf("[Skills] Detected change in %s, reloading...\n", skill.manifest.Name)
			w.registry.Reload(skill.manifest.Name)
		}
	}
}
