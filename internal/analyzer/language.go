package analyzer

import "github.com/vzx7/opencode-mcp/internal/domain"

// ProjectAnalyzer encapsulates all language-specific analysis operations.
// Implement this interface to add support for a new programming language.
type ProjectAnalyzer interface {
	// Name returns the language identifier ("go", "python", "typescript").
	Name() string

	// Detect returns true if this analyzer can handle the project at rootPath.
	Detect(rootPath string) bool

	// SourceExtensions returns file extensions for source files (e.g., []string{".go"}).
	SourceExtensions() []string

	// IsTestFile returns true if the given file path is a test file.
	IsTestFile(path string) bool

	// HasTestCounterpart returns true if a test file exists alongside the given source file.
	HasTestCounterpart(sourcePath string) bool

	// BuildImportGraph constructs the intra-project import/dependency graph (edges only).
	// Cycles and layer violations are added by the Engine after this call.
	BuildImportGraph(rootPath string) (*domain.ImportGraph, error)

	// CountSymbols counts publicly exported functions and types in a source file.
	CountSymbols(src []byte) (functions, types int)

	// SnippetLang returns the language tag for markdown code blocks ("go", "python").
	SnippetLang() string

	// ModuleName extracts the project module/package name from the project manifest.
	// Returns empty string if not applicable.
	ModuleName(rootPath string) string
}
