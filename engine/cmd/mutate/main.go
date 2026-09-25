// Command mutate is the K3 mutation-test driver (plan 14 §8.1, §9 K3).
//
//	mutate gen [-op name]… [-out dir] model.pml
//	        write every first-order mutant of the model into dir together
//	        with manifest.json; without -out, print the manifest on stdout
//	        and write nothing.
//
//	mutate run [-op name]… [-out dir] [-mcd path] [-spin path] [-gcc path]
//	           [-jobs n] [-timeout s] [-config file] [-json file] [-md file]
//	        generate the mutants of every model of the configuration, run the
//	        engine (through the `mcd` binary) and SPIN's pan on the original
//	        and on every mutant, classify each mutant and print the
//	        detection rate.
//
// The engine is reached through the CLI binary, never through its packages:
// this step runs beside G5's changes to the engine and must not break when a
// package API moves.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"modelcheck/tools/mutate"
)

type opList []mutate.Operator

func (o *opList) String() string { return "" }

func (o *opList) Set(s string) error {
	op, err := mutate.ParseOperator(s)
	if err != nil {
		return err
	}
	*o = append(*o, op)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "gen":
		os.Exit(gen(os.Args[2:]))
	case "run":
		os.Exit(run(os.Args[2:]))
	case "corpus2":
		os.Exit(corpus2(os.Args[2:]))
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mutate (gen|run|corpus2) [flags] …")
	fmt.Fprintln(os.Stderr, "operators:", strings.Join(operatorNames(), ", "))
}

func operatorNames() []string {
	out := make([]string, 0, len(mutate.Operators))
	for _, op := range mutate.Operators {
		out = append(out, string(op))
	}
	return out
}

func gen(args []string) int {
	fs := flag.NewFlagSet("mutate gen", flag.ContinueOnError)
	var ops opList
	fs.Var(&ops, "op", "mutation operator to apply (repeatable; default: all)")
	out := fs.String("out", "", "directory for the mutants and manifest.json (default: print the manifest only)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "mutate gen: exactly one model file")
		return 2
	}
	path := fs.Arg(0)
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutate gen:", err)
		return 1
	}
	muts := mutate.Generate(src, path, ops...)
	if *out == "" {
		entries := make([]mutate.Entry, 0, len(muts))
		for _, m := range muts {
			e := m.Entry
			e.File = m.FileName()
			entries = append(entries, e)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(entries); err != nil {
			fmt.Fprintln(os.Stderr, "mutate gen:", err)
			return 1
		}
		return 0
	}
	manifest, err := mutate.WriteAll(*out, muts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutate gen:", err)
		return 1
	}
	fmt.Printf("%d mutants, manifest %s\n", len(muts), manifest)
	return 0
}

func run(args []string) int {
	fs := flag.NewFlagSet("mutate run", flag.ContinueOnError)
	var ops opList
	fs.Var(&ops, "op", "mutation operator to apply (repeatable; default: all)")
	cfg := fs.String("config", "", "campaign configuration JSON (default: the built-in subset campaign)")
	outDir := fs.String("out", "", "directory for the mutants (required)")
	mcd := fs.String("mcd", "mcd", "the engine's CLI binary")
	spin := fs.String("spin", "spin", "the spin executable")
	gcc := fs.String("gcc", "gcc", "the C compiler for pan.c")
	root := fs.String("corpus", "", "root of the model corpus that the configuration's paths are relative to")
	jobs := fs.Int("jobs", 4, "parallel mutants")
	timeout := fs.Int("timeout", 120, "per-run timeout in seconds")
	jsonOut := fs.String("json", "", "write the full results as JSON to this file")
	mdOut := fs.String("md", "", "write the report as Markdown to this file")
	keep := fs.String("keep-disagreements", "", "directory that keeps a copy of every class (iii) mutant")
	keepPrefix := fs.String("keep-prefix", "", "path prefix to record for the kept mutants instead of -keep-disagreements")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *outDir == "" {
		fmt.Fprintln(os.Stderr, "mutate run: -out is required")
		return 2
	}
	campaign := mutate.SubsetCampaign()
	if *cfg != "" {
		data, err := os.ReadFile(*cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mutate run:", err)
			return 1
		}
		if err := json.Unmarshal(data, &campaign); err != nil {
			fmt.Fprintln(os.Stderr, "mutate run:", err)
			return 1
		}
	}
	res, err := mutate.RunCampaign(context.Background(), mutate.CampaignOptions{
		Models:     campaign,
		Operators:  ops,
		CorpusRoot: *root,
		OutDir:     *outDir,
		MCD:        *mcd,
		Spin:       *spin,
		GCC:        *gcc,
		Jobs:       *jobs,
		TimeoutSec: *timeout,
		Progress:   os.Stderr,
		KeepDir:    *keep,
		KeepPrefix: *keepPrefix,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutate run:", err)
		return 1
	}
	if *jsonOut != "" {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "mutate run:", err)
			return 1
		}
		if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "mutate run:", err)
			return 1
		}
	}
	md := res.Markdown()
	if *mdOut != "" {
		if err := os.WriteFile(*mdOut, []byte(md), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "mutate run:", err)
			return 1
		}
	}
	fmt.Print(md)
	return 0
}

func corpus2(args []string) int {
	fs := flag.NewFlagSet("mutate corpus2", flag.ContinueOnError)
	root := fs.String("root", "testdata/corpus2", "root of the second corpus")
	mcd := fs.String("mcd", "mcd", "the engine's CLI binary")
	spin := fs.String("spin", "spin", "the spin executable")
	gcc := fs.String("gcc", "gcc", "the C compiler for pan.c")
	timeout := fs.Int("timeout", 120, "per-run timeout in seconds")
	jsonOut := fs.String("json", "", "write the full results as JSON to this file")
	mdOut := fs.String("md", "", "write the README table to this file")
	title := fs.String("title", "Second corpus (plan 14 §2.3)", "title of the generated README")
	preamble := fs.String("preamble", "", "file whose contents go above the table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	pre := ""
	if *preamble != "" {
		data, err := os.ReadFile(*preamble)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mutate corpus2:", err)
			return 1
		}
		pre = string(data)
	}
	res, err := mutate.RunCorpus2(context.Background(), mutate.CampaignOptions{
		MCD: *mcd, Spin: *spin, GCC: *gcc, TimeoutSec: *timeout, Progress: os.Stderr,
	}, *root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mutate corpus2:", err)
		return 1
	}
	if *jsonOut != "" {
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "mutate corpus2:", err)
			return 1
		}
		if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "mutate corpus2:", err)
			return 1
		}
	}
	md := res.Markdown(*title, pre)
	if *mdOut != "" {
		if err := os.WriteFile(*mdOut, []byte(md), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "mutate corpus2:", err)
			return 1
		}
	}
	fmt.Print(md)
	return 0
}
