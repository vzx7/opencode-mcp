package typescript

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

type Analyzer struct{}

func (a *Analyzer) Name() string { return "typescript" }

func (a *Analyzer) Detect(rootPath string) bool {
	for _, marker := range []string{"tsconfig.json", "tsconfig.base.json"} {
		if _, err := os.Stat(filepath.Join(rootPath, marker)); err == nil {
			return true
		}
	}
	return false
}

func (a *Analyzer) SourceExtensions() []string { return []string{".ts", ".tsx"} }

func (a *Analyzer) IsTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.tsx")
}

func (a *Analyzer) HasTestCounterpart(sourcePath string) bool { return false }

func (a *Analyzer) SnippetLang() string { return "typescript" }

func (a *Analyzer) ModuleName(rootPath string) string {
	// TODO: read "name" from package.json
	return ""
}

func (a *Analyzer) BuildImportGraph(rootPath string) (*domain.ImportGraph, error) {
	// TODO: parse ES module imports (import X from 'Y', require('Y'))
	// Filter to intra-project paths (relative or tsconfig path aliases).
	return &domain.ImportGraph{
		Edges:           make(map[string][]string),
		Cycles:          [][]string{},
		LayerViolations: []domain.LayerViolation{},
	}, nil
}

func (a *Analyzer) CountSymbols(src []byte) (int, int) {
	// TODO: count exported functions and interfaces/types
	return 0, 0
}
