package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func findPromptsPath() string {
	paths := []string{
		"internal/llm/prompts",
		"prompts",
	}

	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(p, "en.json")); err == nil {
			return p
		}
	}

	cwd, _ := os.Getwd()
	for _, p := range paths {
		fullPath := filepath.Join(cwd, p)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	for _, p := range paths {
		fullPath := filepath.Join(execDir, p)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	return "internal/llm/prompts"
}

var promptsBasePath = findPromptsPath()

type Prompts struct {
	SystemRole             string            `json:"system_role"`
	Labels                map[string]string `json:"labels"`
	ReviewFocus           []string         `json:"review_focus"`
	JSONSchemaInstruction string           `json:"json_schema_instruction"`
	JSONSchema           JSONSchema       `json:"json_schema"`
	Compliance           CompliancePrompts `json:"compliance"`
	ModuleAudit          ModuleAuditPrompts `json:"module_audit"`
}

type CompliancePrompts struct {
	SystemRole          string `json:"system_role"`
	IdentifyViolations  string `json:"identify_violations"`
	ReturnFormat        string `json:"return_format"`
	SeverityGuide       string `json:"severity_guide"`
}

type ModuleAuditPrompts struct {
	SystemRole    string   `json:"system_role"`
	ReturnFormat  string   `json:"return_format"`
	Focus         []string `json:"focus"`
	ScoringRubric string   `json:"scoring_rubric"`
}

type JSONSchema struct {
	Score          string       `json:"score"`
	Summary       string       `json:"summary"`
	Issues        []IssueSchema `json:"issues"`
	Recommendations []string     `json:"recommendations"`
}

type IssueSchema struct {
	Severity    string `json:"severity"`
	Message    string `json:"message"`
	Location   string `json:"location"`
	Suggestion string `json:"suggestion"`
}

var promptsByLang = map[string]*Prompts{}

func init() {
	loadPromptFile("en", filepath.Join(promptsBasePath, "en.json"))
	loadPromptFile("ru", filepath.Join(promptsBasePath, "ru.json"))
	loadPromptFile("zh", filepath.Join(promptsBasePath, "zh.json"))
}

func loadPromptFile(lang string, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("warning: failed to load prompts for %s: %v\n", lang, err)
		return
	}
	var p Prompts
	if err := json.Unmarshal(data, &p); err != nil {
		fmt.Printf("warning: failed to parse prompts for %s: %v\n", lang, err)
		return
	}
	promptsByLang[lang] = &p
}

func GetPrompts(language string) *Prompts {
	if p, ok := promptsByLang[language]; ok {
		return p
	}
	return promptsByLang["en"]
}

func GetJSONSchema(language string) string {
	p := GetPrompts(language)
	if p == nil || p.JSONSchema.Score == "" {
		return `{
  "score": "<score>",
  "summary": "<summary>",
  "issues": [
    {"severity": "<severity>", "message": "<message>", "location": "<location>", "suggestion": "<suggestion>"}
  ],
  "recommendations": ["<recommendation>"]
}`
	}
	schema := JSONSchema{
		Score:   p.JSONSchema.Score,
		Summary: p.JSONSchema.Summary,
		Issues: []IssueSchema{
			{Severity: "<severity>", Message: "<message>", Location: "<location>", Suggestion: "<suggestion>"},
		},
		Recommendations: []string{"<recommendation>"},
	}
	data, _ := json.MarshalIndent(schema, "", "  ")
	return strings.ReplaceAll(string(data), "\n", "\n")
}

func GetLangLabel(lang string) string {
	switch lang {
	case "ru":
		return "Russian"
	case "zh":
		return "Chinese"
	default:
		return "English"
	}
}