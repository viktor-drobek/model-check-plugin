// Command spike runs the calibration explorer on one hard-coded model and
// prints states/s, bytes per stored state and peak RSS. Throwaway (plan 14
// §9, Spike). Usage:
//
//	spike -model 'counters(K=10, N=6)' [-compact] [-continue] [-json]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"modelcheck/internal/spike"
)

func main() {
	model := flag.String("model", "petrinet1", "petrinet1 | mutex_flaw | counters(K=..., N=...)")
	compact := flag.Bool("compact", false, "use the arena hash table instead of map[string]struct{}")
	cont := flag.Bool("continue", false, "continue after the first violation (pan -c0)")
	asJSON := flag.Bool("json", false, "print the deterministic JSON report")
	maxStates := flag.Int("max-states", 0, "state budget (0 = none)")
	timeout := flag.Duration("timeout", 0, "time budget (0 = none)")
	flag.Parse()

	m, err := spike.ByName(*model)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// Memory: GC before the run and read HeapAlloc; run; drop everything but
	// the visited set; GC again and read HeapAlloc. The delta is the memory
	// retained by the stored states (comparable to pan's "actual memory
	// usage for states"). The DFS stack is transient and shows up only in
	// peak RSS and in the "peak heap" figure taken right after the run.
	debug.SetGCPercent(100)
	runtime.GC()
	var before, peak, after runtime.MemStats
	runtime.ReadMemStats(&before)
	res, st := spike.Explore(m, spike.Options{
		CompactSet: *compact, ContinueAfterViolation: *cont,
		MaxStates: *maxStates, Timeout: *timeout,
	})
	runtime.ReadMemStats(&peak)
	visited := st.Visited
	st.Visited = nil
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(visited)

	if *asJSON {
		os.Stdout.Write(res.JSON())
	}
	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	peakHeap := int64(peak.HeapAlloc) - int64(before.HeapAlloc)
	fmt.Printf("model=%s status=%s evidence=%s violation=%s states=%d transitions=%d depth=%d\n",
		res.Model, res.Status, res.Evidence, res.Violation, res.States, res.Transitions, res.MaxDepth)
	fmt.Printf("elapsed=%s states/s=%.0f state_len=%d set=%s\n",
		st.Elapsed.Round(time.Millisecond), st.StatesPerSec, st.StateLen, setName(*compact))
	fmt.Printf("retained_visited=%d bytes/state=%.1f peak_heap_unGCd=%d (%.1f/state) total_alloc=%d peak_rss_kb=%d\n",
		retained, float64(retained)/float64(res.States), peakHeap, float64(peakHeap)/float64(res.States),
		after.TotalAlloc-before.TotalAlloc, vmHWM())
}

func setName(compact bool) string {
	if compact {
		return "compact"
	}
	return "map"
}

// vmHWM reads the peak resident set size from /proc/self/status (Linux).
func vmHWM() int {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return -1
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "VmHWM:") {
			fs := strings.Fields(sc.Text())
			if len(fs) >= 2 {
				n, _ := strconv.Atoi(fs[1])
				return n
			}
		}
	}
	return -1
}
