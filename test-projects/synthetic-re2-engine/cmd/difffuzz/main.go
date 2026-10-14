// Command difffuzz compares the native engine with the isolated C++ RE2 oracle
// over the deliberately supported grammar. It never runs an LLM candidate.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"

	"syntheticre2/internal/rex"
)

func main() {
	seed := flag.Int64("seed", 20261013, "seed")
	n := flag.Int("max-cases", 200, "case limit")
	oracle := flag.String("oracle-bin", "/tmp/synthetic-re2-oracle", "C++ RE2 helper")
	flag.Parse()
	r := rand.New(rand.NewSource(*seed))
	for i := 0; i < *n; i++ {
		p, s := pattern(r, 3), text(r, 8)
		want, compileOK, err := re2Span(*oracle, p, s)
		if err != nil {
			panic(err)
		}
		got, ge := rex.Compile(p)
		if compileOK != (ge == nil) {
			panic(fmt.Sprintf("compile mismatch seed=%d case=%d p=%q C++RE2=%v engine=%v", *seed, i, p, compileOK, ge))
		}
		if !compileOK {
			continue
		}
		gm := got.Find([]byte(s))
		if (want[0] >= 0) != gm.Matched || (gm.Matched && gm.Span != want) {
			panic(fmt.Sprintf("match mismatch seed=%d case=%d p=%q s=%q C++RE2=%v engine=%+v", *seed, i, p, s, want, gm))
		}
	}
	fmt.Printf("difffuzz: %d C++ RE2 cases passed (seed %d)\n", *n, *seed)
}

func re2Span(bin, pattern, text string) ([2]int, bool, error) {
	out, err := exec.Command(bin, pattern, text).Output()
	if err != nil {
		return [2]int{}, false, fmt.Errorf("run C++ RE2 oracle: %w", err)
	}
	fields := strings.Fields(string(out))
	switch fields[0] {
	case "ERROR":
		return [2]int{-1, -1}, false, nil
	case "NO":
		return [2]int{-1, -1}, true, nil
	case "MATCH":
		if len(fields) != 3 {
			return [2]int{}, false, fmt.Errorf("bad C++ RE2 output %q", out)
		}
		start, err := strconv.Atoi(fields[1])
		if err != nil {
			return [2]int{}, false, err
		}
		width, err := strconv.Atoi(fields[2])
		if err != nil {
			return [2]int{}, false, err
		}
		return [2]int{start, start + width}, true, nil
	default:
		return [2]int{}, false, fmt.Errorf("bad C++ RE2 output %q", out)
	}
}

func pattern(r *rand.Rand, d int) string {
	atoms := []string{"a", "b", "c", ".", "[ab]", "\\d"}
	x := atoms[r.Intn(len(atoms))]
	if d > 0 && r.Intn(3) == 0 {
		x = "(" + x + "|" + atoms[r.Intn(len(atoms))] + ")"
	}
	switch r.Intn(4) {
	case 0:
		return x + "*"
	case 1:
		return x + "+"
	case 2:
		return x + "?"
	}
	return x
}
func text(r *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = "abc123"[r.Intn(6)]
	}
	return string(b)
}
