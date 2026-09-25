package mutate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"modelcheck/tools/pandiff"
)

// The second corpus (plan 14 §2.3). Its point is that the engine was tuned on
// one author's models and may have inherited his style as a hidden norm, so
// the acceptance rule of §2.3 asks that at least a third of the differential
// set come from elsewhere. This file measures each extracted listing the same
// way for both sides and says plainly which of the three things happened:
// the frontend took it, the frontend refused it (and with which kind), or
// SPIN itself refused it — in which case neither side can be right about it.
//
// A listing is never edited to make it pass. The extraction repaired OCR line
// breaks and nothing else (see the header comment of each file).

// Listing outcomes, as the README table prints them.
const (
	EngineParsed  = "parsed"
	EngineOutside = "outside-subset"
	EngineSyntax  = "syntax"
	EngineSemant  = "semantic"
	PanAccepted   = "accepted"
	PanRejected   = "rejected"
)

// ListingResult is one second-corpus file, measured.
type ListingResult struct {
	File        string       `json:"file"`
	SourceLines string       `json:"source_lines"`
	Constructs  []string     `json:"constructs"`
	Engine      string       `json:"engine"`
	EngineNote  string       `json:"engine_note,omitempty"`
	Pan         string       `json:"pan"`
	PanNote     string       `json:"pan_note,omitempty"`
	Diff        *CheckResult `json:"differential,omitempty"`
	Agree       string       `json:"agree"`
}

// Corpus2Results is the whole second corpus.
type Corpus2Results struct {
	Engine   string          `json:"engine"`
	Spin     string          `json:"spin"`
	Started  string          `json:"started"`
	Listings []ListingResult `json:"listings"`
}

// InDifferentialSet reports whether this listing is part of the differential
// set: both sides ran on it and produced a comparable answer. A listing one
// side refuses is a test of refusal, counted in the README and not in the set
// (plan §2.3 asks that refusals not be dropped silently, not that they be
// counted as agreements).
func (l ListingResult) InDifferentialSet() bool {
	return l.Engine == EngineParsed && l.Pan == PanAccepted && l.Diff != nil &&
		isVerdict(l.Diff.Engine) && isVerdict(l.Diff.Pan)
}

// OnlyV1Construct reports whether the single thing standing between this
// listing and the differential set is a construct plan §4 puts in v1 (the
// G5 step): SPIN accepts it and the frontend refused it as outside-subset.
// Counting these separately answers "how much of the second corpus becomes
// available once G5 lands" without claiming it already has.
func (l ListingResult) OnlyV1Construct() bool {
	return l.Engine == EngineOutside && l.Pan == PanAccepted
}

var reSource = regexp.MustCompile(`\*\s*Source:\s*(\S+),\s*lines?\s*([0-9,\-–\s]+?)\s*\(`)

// constructTable drives the "constructs" column. The point of the column is
// to let a reader see WHY a listing is in or out of the subset without
// opening it, so each entry is a construct the frontend either implements or
// names when it refuses.
var constructTable = []struct {
	name string
	re   *regexp.Regexp
}{
	{"chan", regexp.MustCompile(`\bchan\b`)},
	{"rendezvous", regexp.MustCompile(`\[\s*0\s*\]\s*of`)},
	{"chan array", regexp.MustCompile(`\bchan\s+\w+\s*\[`)},
	{"chan param", regexp.MustCompile(`\(\s*chan\b|;\s*chan\b`)},
	{"mtype", regexp.MustCompile(`\bmtype\b`)},
	{"typedef", regexp.MustCompile(`\btypedef\b`)},
	{"inline", regexp.MustCompile(`\binline\b`)},
	{"provided", regexp.MustCompile(`\bprovided\b`)},
	{"unless", regexp.MustCompile(`\bunless\b`)},
	{"atomic", regexp.MustCompile(`\batomic\b`)},
	{"d_step", regexp.MustCompile(`\bd_step\b`)},
	{"never claim", regexp.MustCompile(`\bnever\b`)},
	{"accept label", regexp.MustCompile(`\baccept\w*\s*:`)},
	{"progress label", regexp.MustCompile(`\bprogress\w*\s*:`)},
	{"end label", regexp.MustCompile(`(^|[\s{;:])end\w*\s*:`)},
	{"run", regexp.MustCompile(`\brun\b`)},
	{"active", regexp.MustCompile(`\bactive\b`)},
	{"array", regexp.MustCompile(`\b(byte|bit|bool|int|short|mtype)\s+\w+\s*\[`)},
	{"assert", regexp.MustCompile(`\bassert\b`)},
	{"printf", regexp.MustCompile(`\bprintf\b`)},
	{"timeout", regexp.MustCompile(`\btimeout\b`)},
	{"eval", regexp.MustCompile(`\beval\s*\(`)},
	{"remote label ref", regexp.MustCompile(`\w+\s*\[[^\]]*\]\s*@`)},
	{"directives", regexp.MustCompile(`(?m)^\s*#`)},
	{"non-ASCII identifier", regexp.MustCompile(`[^\x00-\x7F]`)},
}

// Constructs names the constructs a listing uses. The comments are stripped
// first: a russian comment must not make an ASCII listing look non-ASCII, and
// the word `atomic` in a comment is not an atomic block.
func Constructs(src []byte) []string {
	code := stripComments(src)
	var out []string
	for _, c := range constructTable {
		if c.re.Match(code) {
			out = append(out, c.name)
		}
	}
	return out
}

func stripComments(src []byte) []byte {
	var out []byte
	for i := 0; i < len(src); {
		if src[i] == '/' && i+1 < len(src) && src[i+1] == '*' {
			j := i + 2
			for j+1 < len(src) && !(src[j] == '*' && src[j+1] == '/') {
				if src[j] == '\n' {
					out = append(out, '\n')
				}
				j++
			}
			i = j + 2
			continue
		}
		if src[i] == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		out = append(out, src[i])
		i++
	}
	return out
}

// RunCorpus2 measures every .pml under root.
func RunCorpus2(ctx context.Context, o CampaignOptions, root string) (*Corpus2Results, error) {
	if o.MCD == "" {
		o.MCD = "mcd"
	}
	if o.Spin == "" {
		o.Spin = "spin"
	}
	if o.GCC == "" {
		o.GCC = "gcc"
	}
	if o.TimeoutSec <= 0 {
		o.TimeoutSec = 120
	}
	tools := pandiff.Tools{Spin: o.Spin, GCC: o.GCC}
	if !tools.Available() {
		return nil, fmt.Errorf("spin or gcc not found")
	}
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(p) == ".pml" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	res := &Corpus2Results{
		Engine:  firstLine(runOut(ctx, o.MCD, "version")),
		Spin:    firstLine(runOut(ctx, o.Spin, "-V")),
		Started: time.Now().UTC().Format(time.RFC3339),
	}
	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		l := ListingResult{File: filepath.ToSlash(rel), Constructs: Constructs(src), Agree: "—"}
		if m := reSource.FindSubmatch(src); m != nil {
			l.SourceLines = strings.Join(strings.Fields(string(m[2])), " ")
		}
		l.Engine, l.EngineNote = engineParse(ctx, o, f)
		l.Pan, l.PanNote = panAccepts(ctx, tools, f)
		if l.Engine == EngineParsed && l.Pan == PanAccepted {
			cr := CompareOne(ctx, o, f, nil, naturalCheck(src))
			l.Diff = &cr
			switch {
			case !isVerdict(cr.Engine) || !isVerdict(cr.Pan):
				l.Agree = "not comparable"
			case cr.Engine == cr.Pan && cr.EngineStates == cr.PanStates:
				l.Agree = "yes"
			case cr.Engine == cr.Pan:
				l.Agree = fmt.Sprintf("verdict yes, states %d vs %d", cr.EngineStates, cr.PanStates)
			default:
				l.Agree = "NO"
			}
		}
		res.Listings = append(res.Listings, l)
		progressf(o, "%s: engine %s, pan %s, %s\n", l.File, l.Engine, l.Pan, l.Agree)
	}
	return res, nil
}

var reNever = regexp.MustCompile(`(?m)^\s*never\s*{`)

// naturalCheck picks the one check to compare a listing under, by the same
// rule the corpus campaign uses (see corpus.go): a model that carries a never
// claim is compared with `pan -a`, because plain `pan` disables
// invalid-end-state checking under a claim and limits assertions to the
// claim's scope — the two sides would be answering different questions and
// every deadlocking claim model would look like a disagreement. Everything
// else is compared on its asserts and end states.
func naturalCheck(src []byte) Check {
	if reNever.Match(stripComments(src)) {
		return Check{Mode: "a"}
	}
	return Check{Name: "safety"}
}

// engineParse runs `mcd parse` and reduces it to the README's engine column.
func engineParse(ctx context.Context, o CampaignOptions, path string) (string, string) {
	cctx, cancel := context.WithTimeout(ctx, time.Duration(o.TimeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, o.MCD, "parse", "--promela", path)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	_ = cmd.Run()
	switch cmd.ProcessState.ExitCode() {
	case 0:
		return EngineParsed, ""
	case 2:
		var rej struct {
			Error struct {
				Kind, Message string
			} `json:"error"`
		}
		if json.Unmarshal([]byte(out.String()), &rej) == nil && rej.Error.Kind != "" {
			return rej.Error.Kind, trim(rej.Error.Message, 160)
		}
		return "rejected", trim(out.String(), 160)
	}
	return "tool error", trim(errb.String(), 160)
}

// panAccepts asks whether SPIN itself takes the listing.
func panAccepts(ctx context.Context, tools pandiff.Tools, path string) (string, string) {
	dir, err := os.MkdirTemp("", "k3c2-")
	if err != nil {
		return "tool error", err.Error()
	}
	defer os.RemoveAll(dir)
	src, err := os.ReadFile(path)
	if err != nil {
		return "tool error", err.Error()
	}
	if err := os.WriteFile(filepath.Join(dir, "m.pml"), src, 0o644); err != nil {
		return "tool error", err.Error()
	}
	out, err := runIn(ctx, dir, tools.Spin, "-a", "-o1", "-o2", "-o3", "m.pml")
	if err != nil || strings.Contains(out, "Error") {
		return PanRejected, trim(out, 160)
	}
	if out, err := runIn(ctx, dir, tools.GCC, "-O2", "-DNOREDUCE", "-o", "pan", "pan.c"); err != nil {
		return PanRejected, "gcc: " + trim(out, 160)
	}
	return PanAccepted, ""
}

// Markdown renders the corpus2 README table.
func (r *Corpus2Results) Markdown(title, preamble string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n", title, preamble)
	fmt.Fprintf(&b, "Measured with `%s` and `%s` on %s.\n\n", r.Engine, r.Spin, r.Started)
	b.WriteString("| file | source lines | constructs | engine | pan | agree? |\n|---|---|---|---|---|---|\n")
	for _, l := range r.Listings {
		engine := l.Engine
		if l.Engine != EngineParsed && l.EngineNote != "" {
			engine = l.Engine + " — " + l.EngineNote
		}
		pan := l.Pan
		if l.Pan == PanRejected && l.PanNote != "" {
			pan = l.Pan + " — " + l.PanNote
		}
		agree := l.Agree
		if l.Diff != nil {
			agree = fmt.Sprintf("%s (engine %s/%d, pan %s/%d)", l.Agree, l.Diff.Engine, l.Diff.EngineStates, l.Diff.Pan, l.Diff.PanStates)
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s |\n",
			l.File, l.SourceLines, strings.Join(l.Constructs, ", "), engine, pan, agree)
	}
	inSet, onlyV1, spinRejects := 0, 0, 0
	for _, l := range r.Listings {
		switch {
		case l.InDifferentialSet():
			inSet++
		case l.OnlyV1Construct():
			onlyV1++
		case l.Pan == PanRejected:
			spinRejects++
		}
	}
	fmt.Fprintf(&b, "\n%d listings: %d in the differential set, %d refused by the frontend as outside the subset while SPIN accepts them, %d refused by SPIN itself, %d otherwise not comparable.\n",
		len(r.Listings), inSet, onlyV1, spinRejects, len(r.Listings)-inSet-onlyV1-spinRejects)
	return b.String()
}
