package skills

import (
	"regexp"
	"strings"
)

var frontMatterRegex = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`)

func ParseSKILLMd(content string) (*SkillManifest, error) {
	matches := frontMatterRegex.FindStringSubmatch(content)
	if len(matches) < 3 {
		return parseSimpleFormat(content)
	}

	frontMatter := matches[1]
	body := strings.TrimSpace(matches[2])

	manifest := &SkillManifest{
		Metadata: make(map[string]any),
		Tools:    []ToolDefinition{},
		Prompts:  make(map[string]string),
		Scripts:  make(map[string]string),
	}

	lines := strings.Split(frontMatter, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "name":
			manifest.Name = value
		case "description":
			manifest.Description = value
		case "version":
			manifest.Version = value
		case "author":
			manifest.Author = value
		case "homepage":
			manifest.Homepage = value
		case "emoji":
			manifest.Emoji = value
		case "os":
			manifest.OS = parseList(value)
		case "metadata":
			if strings.HasPrefix(value, "{") {
				parseMetadata(manifest, value)
			}
		}
	}

	if manifest.Description == "" {
		manifest.Description = extractFirstParagraph(body)
	}

	parseToolsFromBody(manifest, body)

	return manifest, nil
}

func parseSimpleFormat(content string) (*SkillManifest, error) {
	lines := strings.Split(content, "\n")
	manifest := &SkillManifest{
		Metadata: make(map[string]any),
		Tools:    []ToolDefinition{},
		Prompts:  make(map[string]string),
		Scripts:  make(map[string]string),
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			manifest.Name = strings.TrimPrefix(line, "# ")
			continue
		}
		if manifest.Name != "" && manifest.Description == "" && line != "" {
			manifest.Description = line
			break
		}
	}

	if manifest.Description == "" {
		manifest.Description = extractFirstParagraph(content)
	}

	parseToolsFromBody(manifest, content)

	return manifest, nil
}

func parseList(value string) []string {
	value = strings.Trim(value, "[]")
	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"`)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func parseMetadata(manifest *SkillManifest, value string) {
	value = strings.Trim(value, "{}")
	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, ":", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(strings.Trim(kv[0], `"`))
			val := strings.TrimSpace(strings.Trim(kv[1], `"`))
			manifest.Metadata[key] = val
		}
	}
}

func extractFirstParagraph(body string) string {
	lines := strings.Split(body, "\n")
	var para []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, line)
	}
	return strings.Join(para, " ")
}

func parseToolsFromBody(manifest *SkillManifest, body string) {
	codeBlockRegex := regexp.MustCompile("```(\\w+)(?:\\s+(\\S+))?\\n([\\s\\S]*?)```")
	matches := codeBlockRegex.FindAllStringSubmatch(body, -1)

	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		lang := match[1]
		name := match[2]
		code := strings.TrimSpace(match[3])

		if lang == "bash" || lang == "sh" {
			if name == "" {
				name = manifest.Name + "_script"
			}
			manifest.Scripts[name] = code
		}

		if lang == "prompt" || lang == "text" {
			if name == "" {
				name = "default"
			}
			manifest.Prompts[name] = code
		}
	}

	toolSectionRegex := regexp.MustCompile(`(?i)## tools?\s*\n([\s\S]*?)(?:##|$)`)
	toolMatch := toolSectionRegex.FindStringSubmatch(body)
	if len(toolMatch) > 1 {
		parseToolSection(manifest, toolMatch[1])
	}
}

func parseToolSection(manifest *SkillManifest, section string) {
	lines := strings.Split(section, "\n")
	var currentTool *ToolDefinition

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "- **") {
			if currentTool != nil {
				manifest.Tools = append(manifest.Tools, *currentTool)
			}

			name := ""
			if strings.HasPrefix(line, "### ") {
				name = strings.TrimPrefix(line, "### ")
			} else {
				name = strings.TrimPrefix(line, "- **")
				name = strings.TrimSuffix(name, "**")
				if idx := strings.Index(name, "**"); idx > 0 {
					name = name[:idx]
				}
			}

			name = strings.TrimSpace(name)
			if name != "" {
				currentTool = &ToolDefinition{Name: name}
			}
		}

		if currentTool != nil && strings.HasPrefix(line, "- ") {
			desc := strings.TrimPrefix(line, "- ")
			if currentTool.Description == "" {
				currentTool.Description = desc
			}
		}
	}

	if currentTool != nil {
		manifest.Tools = append(manifest.Tools, *currentTool)
	}
}
