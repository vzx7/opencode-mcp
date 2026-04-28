package golang

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

type Analyzer struct{}

func (a *Analyzer) Name() string { return "go" }

func (a *Analyzer) Detect(rootPath string) bool {
	_, err := os.Stat(filepath.Join(rootPath, "go.mod"))
	return err == nil
}

func (a *Analyzer) SourceExtensions() []string { return []string{".go"} }

func (a *Analyzer) IsTestFile(path string) bool {
	return strings.HasSuffix(filepath.Base(path), "_test.go")
}

func (a *Analyzer) HasTestCounterpart(sourcePath string) bool {
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	_, err := os.Stat(filepath.Join(dir, name+"_test.go"))
	return err == nil
}

func (a *Analyzer) SnippetLang() string { return "go" }

func (a *Analyzer) ModuleName(rootPath string) string {
	data, err := os.ReadFile(filepath.Join(rootPath, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// BuildImportGraph builds the intra-project import graph (edges only).
// Cycles and layer violations are populated by the Engine after this call.
func (a *Analyzer) BuildImportGraph(rootPath string) (*domain.ImportGraph, error) {
	graph := &domain.ImportGraph{
		Edges:           make(map[string][]string),
		Cycles:          [][]string{},
		LayerViolations: []domain.LayerViolation{},
	}

	modulePrefix := a.ModuleName(rootPath)
	fset := token.NewFileSet()

	err := filepath.Walk(rootPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if a.IsTestFile(path) || filepath.Ext(info.Name()) != ".go" {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		f, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if err != nil {
			return nil
		}

		pkgPath, _ := filepath.Rel(rootPath, filepath.Dir(path))
		pkgPath = filepath.ToSlash(pkgPath)

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if modulePrefix == "" || strings.HasPrefix(importPath, modulePrefix) {
				importPath = strings.TrimPrefix(importPath, modulePrefix)
				importPath = strings.TrimPrefix(importPath, "/")
				graph.Edges[pkgPath] = append(graph.Edges[pkgPath], importPath)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return graph, nil
}

func (a *Analyzer) CountSymbols(src []byte) (functions, types int) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return 0, 0
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name != nil && d.Name.IsExported() {
				functions++
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if s, ok := spec.(*ast.TypeSpec); ok && s.Name != nil && s.Name.IsExported() {
					types++
				}
			}
		}
	}
	return
}
