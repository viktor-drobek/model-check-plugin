package promela

import (
	"path/filepath"
	"strings"

	"modelcheck/ir"
)

// Result is an accepted model.
type Result struct {
	Model    *ir.Model
	Warnings []string
	// Module is the AST (for tools such as pandiff).
	Module *Module
	// PanLines: see Lowered.PanLines.
	PanLines map[string][]int
}

// Parse runs the whole frontend on src: lexer, preprocessor (with -D
// defines), parser, lowering. file is used for origins and messages; the
// model's name is the file's base name without extension.
func Parse(src []byte, file string, defines []string) (*Result, *Error) {
	toks, err := Lex(string(src), file)
	if err != nil {
		return nil, err
	}
	toks, err = Preprocess(toks, defines, file)
	if err != nil {
		return nil, err
	}
	mod, err := ParseTokens(toks, file)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	lw, err := Lower(mod, file, name)
	if err != nil {
		return nil, err
	}
	return &Result{Model: lw.Model, Warnings: lw.Warnings, Module: mod, PanLines: lw.PanLines}, nil
}

// ParseTokens is the parser stage alone.
func ParseTokens(toks []Token, file string) (*Module, *Error) { return parseModule(toks, file) }
