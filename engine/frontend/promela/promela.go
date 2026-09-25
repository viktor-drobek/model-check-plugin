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
	// Defines are the object-like #define macros (including -D symbols),
	// name → body text, for the atoms of LTL formulas.
	Defines map[string]string
}

// Options configures the frontend.
type Options struct {
	// MaxProcs is the size of the instance pool for a proctype whose `run`
	// may be taken more than once; 0 means DefaultMaxProcs.
	MaxProcs int
}

// Parse runs the whole frontend on src with the default options.
func Parse(src []byte, file string, defines []string) (*Result, *Error) {
	return ParseWith(src, file, defines, Options{})
}

// ParseWith runs the whole frontend on src: lexer, preprocessor (with -D
// defines), inline expansion, parser, lowering. file is used for origins
// and messages; the model's name is the file's base name without extension.
func ParseWith(src []byte, file string, defines []string, opt Options) (*Result, *Error) {
	toks, err := Lex(string(src), file)
	if err != nil {
		return nil, err
	}
	toks, macros, err := PreprocessMacros(toks, defines, file)
	if err != nil {
		return nil, err
	}
	toks, err = ExpandInlines(toks, file)
	if err != nil {
		return nil, err
	}
	mod, err := ParseTokens(toks, file)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	lw, err := LowerWith(mod, file, name, opt.MaxProcs)
	if err != nil {
		return nil, err
	}
	return &Result{Model: lw.Model, Warnings: lw.Warnings, Module: mod, PanLines: lw.PanLines, Defines: Defines(macros)}, nil
}

// ParseTokens is the parser stage alone.
func ParseTokens(toks []Token, file string) (*Module, *Error) { return parseModule(toks, file) }
