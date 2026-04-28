package analyzer

import (
	"github.com/vzx7/opencode-mcp/internal/analyzer/golang"
	"github.com/vzx7/opencode-mcp/internal/analyzer/python"
	"github.com/vzx7/opencode-mcp/internal/analyzer/typescript"
)

// registered is the ordered list of available language analyzers.
// Detection runs in order; the first match wins.
var registered = []ProjectAnalyzer{
	&golang.Analyzer{},
	&typescript.Analyzer{},
	&python.Analyzer{},
}

// Detect returns the first analyzer whose Detect() returns true for rootPath.
// Falls back to the Go analyzer if nothing matches.
func Detect(rootPath string) ProjectAnalyzer {
	for _, a := range registered {
		if a.Detect(rootPath) {
			return a
		}
	}
	return &golang.Analyzer{}
}

// ByName returns the analyzer registered under the given name.
// ok is false if no analyzer with that name is registered.
func ByName(name string) (ProjectAnalyzer, bool) {
	for _, a := range registered {
		if a.Name() == name {
			return a, true
		}
	}
	return nil, false
}
