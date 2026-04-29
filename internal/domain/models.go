package domain

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow     Severity = "low"
)

type Issue struct {
	Severity   Severity `json:"severity"`
	Message   string   `json:"message"`
	Location  string   `json:"location"`
	Suggestion string  `json:"suggestion"`
}

type AuditReport struct {
	Score          int      `json:"score"`
	Summary        string   `json:"summary"`
	Issues         []Issue  `json:"issues"`
	Recommendations []string `json:"recommendations"`
}

type ArchitectureRules struct {
	Description  string           `json:"description,omitempty"`
	Layers       []LayerRule      `json:"layers"`
	Dependencies []DependencyRule `json:"forbidden_dependencies"`
	Constraints  []string         `json:"constraints"`
}

type LayerRule struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Paths       []string `json:"patterns"`
	Allow       []string `json:"allow_imports_from"`
}

type DependencyRule struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

type ProjectMap struct {
	Root         string            `json:"root"`
	Modules     []Module         `json:"modules"`
	Entrypoints []string         `json:"entrypoints"`
	Dependencies map[string][]string `json:"dependencies"`
	Layers       []ArchitectureLayer `json:"layers"`
}

type Module struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Files       []string `json:"files"`
	Type        string   `json:"type"`
	SourceFiles int      `json:"source_files"`
}

type ArchitectureLayer struct {
	Name string   `json:"name"`
	Paths []string `json:"paths"`
}

type ImportGraph struct {
	Edges           map[string][]string `json:"edges"`
	Cycles          [][]string          `json:"cycles"`
	LayerViolations []LayerViolation    `json:"layer_violations"`
}

type LayerViolation struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Message string `json:"message"`
}

type FileMetric struct {
	Path           string `json:"path"`
	Lines          int    `json:"lines"`
	ExportedFuncs  int    `json:"exported_funcs"`
	ExportedTypes  int    `json:"exported_types"`
	HasTests       bool   `json:"has_tests"`
}

type GitHotspot struct {
	Path    string `json:"path"`
	Commits int    `json:"commits"`
}
