package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SkillManifest struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Author      string                 `json:"author"`
	Description string                 `json:"description"`
	Permissions []string               `json:"permissions"`
	Config      map[string]ConfigField `json:"config"`
	EntryPoints []string               `json:"entryPoints"`
}

type ConfigField struct {
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     any    `json:"default"`
}

type Skill interface {
	Manifest() *SkillManifest
	Execute(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error)
	Load() error
	Unload() error
}

type BaseSkill struct {
	manifest *SkillManifest
	loaded   bool
	loadTime time.Time
}

func (s *BaseSkill) Manifest() *SkillManifest {
	return s.manifest
}

func (s *BaseSkill) Execute(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error) {
	if !s.loaded {
		return Result{Success: false, Error: "skill not loaded"}, nil
	}
	return Result{Success: true, Data: "base skill"}, nil
}

func (s *BaseSkill) Load() error {
	s.loaded = true
	s.loadTime = time.Now()
	return nil
}

func (s *BaseSkill) Unload() error {
	s.loaded = false
	return nil
}

func (s *BaseSkill) IsLoaded() bool {
	return s.loaded
}

func (s *BaseSkill) LastLoaded() time.Time {
	return s.loadTime
}

type ScriptSkill struct {
	BaseSkill
	Path        string
	ExecuteFunc func(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error)
}

func (s *ScriptSkill) Execute(ctx context.Context, params map[string]any, toolCtx *ToolContext) (Result, error) {
	if !s.loaded {
		return Result{Success: false, Error: "skill not loaded"}, nil
	}
	return s.ExecuteFunc(ctx, params, toolCtx)
}

type SkillLoader struct {
	mu         sync.RWMutex
	skills     map[string]Skill
	skillsDir  string
	reloadCh   chan string
	autoReload bool
}

func NewSkillLoader(skillsDir string) *SkillLoader {
	return &SkillLoader{
		skills:     make(map[string]Skill),
		skillsDir:  skillsDir,
		reloadCh:   make(chan string, 10),
		autoReload: true,
	}
}

func (sl *SkillLoader) LoadSkill(ctx context.Context, path string) (Skill, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("skill path not found: %w", err)
	}

	if info.IsDir() {
		return sl.loadSkillDir(ctx, path)
	}

	return sl.loadSkillFile(ctx, path)
}

func (sl *SkillLoader) loadSkillDir(ctx context.Context, dir string) (Skill, error) {
	manifestPath := filepath.Join(dir, "skill.json")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read skill manifest: %w", err)
	}

	var manifest SkillManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse skill manifest: %w", err)
	}

	skill := &ScriptSkill{
		Path: dir,
	}

	entryFile := filepath.Join(dir, "index.go")
	if _, err := os.Stat(entryFile); err == nil {
		skill.manifest = &manifest
		skill.BaseSkill.manifest = &manifest
	}

	if skill.manifest == nil {
		for _, ep := range manifest.EntryPoints {
			entryFile := filepath.Join(dir, ep)
			if _, err := os.Stat(entryFile); err == nil {
				skill.manifest = &manifest
				skill.BaseSkill.manifest = &manifest
				break
			}
		}
	}

	if skill.manifest == nil {
		return nil, fmt.Errorf("no valid entry point found for skill")
	}

	if err := skill.Load(); err != nil {
		return nil, fmt.Errorf("load skill: %w", err)
	}

	return skill, nil
}

func (sl *SkillLoader) loadSkillFile(ctx context.Context, file string) (Skill, error) {
	ext := filepath.Ext(file)

	switch ext {
	case ".json":
		var manifest SkillManifest
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		skill := &BaseSkill{manifest: &manifest}
		skill.Load()
		return skill, nil

	case ".go":
		return nil, fmt.Errorf("Go skill files require directory with skill.json manifest")

	default:
		return nil, fmt.Errorf("unsupported skill file type: %s", ext)
	}
}

func (sl *SkillLoader) LoadAll(ctx context.Context) error {
	if sl.skillsDir == "" {
		return nil
	}

	entries, err := os.ReadDir(sl.skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(sl.skillsDir, entry.Name())
		skill, err := sl.LoadSkill(ctx, skillPath)
		if err != nil {
			fmt.Printf("failed to load skill %s: %v\n", entry.Name(), err)
			continue
		}

		sl.mu.Lock()
		sl.skills[skill.Manifest().Name] = skill
		sl.mu.Unlock()
	}

	return nil
}

func (sl *SkillLoader) Get(name string) (Skill, bool) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	skill, ok := sl.skills[name]
	return skill, ok
}

func (sl *SkillLoader) List() []*SkillManifest {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	manifests := make([]*SkillManifest, 0, len(sl.skills))
	for _, skill := range sl.skills {
		manifests = append(manifests, skill.Manifest())
	}
	return manifests
}

func (sl *SkillLoader) Unload(name string) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	skill, ok := sl.skills[name]
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}

	if err := skill.Unload(); err != nil {
		return fmt.Errorf("unload skill: %w", err)
	}

	delete(sl.skills, name)
	return nil
}

func (sl *SkillLoader) Reload(name string) error {
	sl.mu.RLock()
	skill, ok := sl.skills[name]
	sl.mu.RUnlock()

	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}

	skillPath := skill.Manifest().Name

	if err := skill.Unload(); err != nil {
		return fmt.Errorf("unload skill: %w", err)
	}

	newSkill, err := sl.LoadSkill(context.Background(), filepath.Join(sl.skillsDir, skillPath))
	if err != nil {
		return fmt.Errorf("reload skill: %w", err)
	}

	sl.mu.Lock()
	sl.skills[name] = newSkill
	sl.mu.Unlock()

	return nil
}

func (sl *SkillLoader) ReloadChannel() <-chan string {
	return sl.reloadCh
}

func (sl *SkillLoader) TriggerReload(name string) {
	sl.reloadCh <- name
}
