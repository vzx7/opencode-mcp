package python

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

type Analyzer struct{}

func (a *Analyzer) Name() string { return "python" }

func (a *Analyzer) Detect(rootPath string) bool {
	for _, marker := range []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"} {
		if _, err := os.Stat(filepath.Join(rootPath, marker)); err == nil {
			return true
		}
	}
	return false
}

func (a *Analyzer) SourceExtensions() []string { return []string{".py"} }

func (a *Analyzer) IsTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
}

func (a *Analyzer) HasTestCounterpart(sourcePath string) bool { return false }

func (a *Analyzer) SnippetLang() string { return "python" }

func (a *Analyzer) ModuleName(rootPath string) string {
	// TODO: parse [project].name from pyproject.toml or name= from setup.py
	return ""
}

func (a *Analyzer) BuildImportGraph(rootPath string) (*domain.ImportGraph, error) {
	// TODO: parse Python imports (import X, from X import Y), filter to intra-project modules
	return &domain.ImportGraph{
		Edges:           make(map[string][]string),
		Cycles:          [][]string{},
		LayerViolations: []domain.LayerViolation{},
	}, nil
}

func (a *Analyzer) CountSymbols(src []byte) (int, int) {
	// TODO: count top-level def (functions) and class (types)
	return 0, 0
}
