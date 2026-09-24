// Command pandiff compares the engine with SPIN's pan on Promela models
// (plan 14 §8.1): verdict, error class, state count with optimisations
// disabled, and the statement table of pan -d. It prints a table per model
// and exits 1 on any disagreement (2 when a tool failed).
//
//	pandiff [-D NAME[=val]]… [-spin path] [-gcc path] [-keep] [-no-statements] model.pml…
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"modelcheck/tools/pandiff"
)

type defineList []string

func (d *defineList) String() string     { return strings.Join(*d, " ") }
func (d *defineList) Set(s string) error { *d = append(*d, s); return nil }

func main() {
	var defines defineList
	flag.Var(&defines, "D", "preprocessor symbol for both spin and the engine (repeatable)")
	spin := flag.String("spin", "spin", "spin executable")
	gcc := flag.String("gcc", "gcc", "C compiler for pan.c")
	keep := flag.Bool("keep", false, "keep the temporary directory with pan.c and outputs")
	noStmts := flag.Bool("no-statements", false, "do not count a statement-table difference as a disagreement")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: pandiff [flags] model.pml…")
		os.Exit(2)
	}
	tools := pandiff.Tools{Spin: *spin, GCC: *gcc}
	if !tools.Available() {
		fmt.Fprintln(os.Stderr, "pandiff: spin or gcc not found on PATH")
		os.Exit(2)
	}
	exit := 0
	for _, model := range flag.Args() {
		cmp, pan, _, err := pandiff.Run(context.Background(), tools, model, defines, *keep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", model, err)
			if pan != nil {
				fmt.Fprintf(os.Stderr, "  pan: %d states stored, class %q\n", pan.Stored, pan.Class)
			}
			exit = 2
			continue
		}
		fmt.Print(cmp.Table(model))
		if !cmp.Agree || (!cmp.StmtAgree && !*noStmts) {
			if exit == 0 {
				exit = 1
			}
		}
	}
	os.Exit(exit)
}
