// Command pormut runs the mutation harness of the partial-order reduction
// (package tools/pormut): the mutants of tools/pormut/mutants.json, one at a
// time, against the oracles of the explore package, after a baseline run of
// the unmutated copy.
//
//	go run ./cmd/pormut -scratch DIR [-models N] [-workers W] [-only id,id] [-list]
//
// Run it under `ulimit -v` and `timeout`: it runs go test, and the oracles
// generate models.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"modelcheck/tools/pormut"
)

func main() {
	var (
		scratch = flag.String("scratch", "", "directory for the copy of the engine tree (required, outside the repository)")
		models  = flag.Int("models", 3000, "models per generator for each oracle layer")
		workers = flag.Int("workers", 1, "goroutines of each oracle run")
		only    = flag.String("only", "", "comma-separated mutant ids (default: all)")
		list    = flag.Bool("list", false, "list the mutants and exit")
		seed    = flag.Int64("seed", 0, "first seed of every generator (0: their defaults)")
		timeout = flag.Duration("timeout", 30*time.Minute, "limit of one oracle layer")
		keep    = flag.Bool("keep", false, "keep the scratch copy")
		details = flag.Bool("details", false, "print the failing lines of every mutant that an oracle killed (with MCD_POR_ALL set: the failure counts)")
		path    = flag.String("mutants", "tools/pormut/mutants.json", "the mutant list")
	)
	flag.Parse()
	ms, err := pormut.LoadMutants(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *list {
		for _, m := range ms {
			fmt.Printf("%-28s %-8s %-10s %s\n", m.ID, m.Rule, m.Expect, m.Note)
		}
		return
	}
	if *scratch == "" {
		fmt.Fprintln(os.Stderr, "pormut: -scratch is required")
		os.Exit(2)
	}
	sel := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id = strings.TrimSpace(id); id != "" {
			sel[id] = true
		}
	}
	cfg := pormut.Config{Models: *models, Workers: *workers, Timeout: *timeout, Seed: *seed, Keep: *keep}
	res, err := pormut.Run(context.Background(), ".", *path, *scratch, sel, cfg, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	text, bad := pormut.Summary(res)
	fmt.Println(text)
	for _, r := range res {
		if (r.Bad || *details) && r.Detail != "" {
			fmt.Printf("\n%s: %s\n%s\n", r.Mutant.ID, r.Outcome, r.Detail)
		}
	}
	if bad > 0 {
		os.Exit(1)
	}
}
