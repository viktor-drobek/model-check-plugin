package explore

import (
	"math"
	"testing"
)

// The floors of the oracle runs say "this generator still exercises the rule":
// a count of the models that came out smaller. A floor of "one in n of the
// models" cannot serve every size of a run. The count of a run of 375 models (go
// test -short) wanders by 7 either side of its mean of about 55, so a floor a
// little below the mean fails on a fresh seed (MCD_POR_SEED, the harness's -seed)
// one run in five, while the same share of 3 000 models, where the count wanders
// by 16 around 440, is safe. A floor is therefore the share of the models that a
// generator is known to reach, less a number of standard deviations of a binomial
// count, so that the chance of a spurious failure is the same at any size
// whatever the seed; and a floor of zero is no check at that size.
//
// Measured on 24 000 fresh seeds per generator (900 000 001 on), over every
// window of 375 and of 3 000 consecutive seeds: the variance of the count of
// smaller models is at most 0.94 of the binomial's in windows of 3 000 and up to
// 1.2 in windows of 375 (the loop generator 1.7 to 3.0: the models of
// consecutive seeds are not independent draws), so the binomial deviation is an
// upper bound at the default size and not at the -short one; that is the reason
// -short runs use half the share (porFloorAt).

const (
	// porFloorSigmas is how many binomial standard deviations below its mean a
	// count of an oracle run may fall: 1e-9 for a binomial count, and the
	// floors of porReducedShare are 7 to 17 observed deviations below the counts
	// at the default size.
	porFloorSigmas = 6
	// porTightSigmas is the same for the floors of the tight-limit run, which
	// have to be close enough to the count to fail when the budget of the proviso
	// is lowered to one (porTightShare): 3e-7 for a binomial count, and the
	// observed deviation of that count is 0.65 to 0.97 of the binomial's at 3 000
	// models, so five binomial deviations are 5.7 to 7.7 observed ones (about 1e-8).
	porTightSigmas = 5
)

// porFloorSlack is how far below its mean a floor is at the least, whatever the
// size of the run: the binomial margin shrinks as the run grows (1.4% of the mean
// at 300 000 models), and a share is an estimate from 24 000 seeds, 1 to 2% off.
const porFloorSlack = 0.05

// porFloorAt is the least a count of the models that have some property may be in
// a run of `models` models, when each has it with probability at least share. A
// run of go test -short counts a share of half that, and a result of zero means
// the run is too small for the floor to say anything.
func porFloorAt(models int, share, sigmas float64, short bool) int {
	if short {
		share /= 2
	}
	mean := float64(models) * share
	f := mean - sigmas*math.Sqrt(mean*(1-share))
	f = math.Min(f, mean*(1-porFloorSlack))
	if f < 0 {
		return 0
	}
	return int(f)
}

// porFloorOf is the floor of the current run.
func porFloorOf(models int, share, sigmas float64) int {
	return porFloorAt(models, share, sigmas, testing.Short())
}

// binomialLowerTail is P(X < k) for X ~ Binomial(n, p), summed exactly in log space.
func binomialLowerTail(n int, p float64, k int) float64 {
	if k <= 0 {
		return 0
	}
	lg := func(x int) float64 { v, _ := math.Lgamma(float64(x) + 1); return v }
	sum := 0.0
	for i := 0; i < k && i <= n; i++ {
		sum += math.Exp(lg(n) - lg(i) - lg(n-i) + float64(i)*math.Log(p) + float64(n-i)*math.Log(1-p))
	}
	return sum
}

// The release bar is 300 000 models per generator, and a floor that is five
// binomial deviations below a share is only 1.4% below it at that size: a share
// that is 1.7% above the true rate fails the run. The first scale run of the
// floors did exactly that: 300 000 models of `loop` under the tight limits gave
// 90 218 smaller models (0.3007) against a floor of 90 239 from the share 0.305
// that 24 000 seeds had given (0.3058). A floor is never closer than 5% to its
// mean (porFloorSlack), whatever the size.
func TestPORFloorAtTheReleaseBarDoesNotFailAMeasuredRun(t *testing.T) {
	for _, c := range []struct {
		name   string
		share  float64
		sigmas float64
		got    int // smaller models in a window of 300 000 seeds
	}{
		{"loop under the tight limits", porTightShare["loop"], porTightSigmas, 90218},
		{"loop", porReducedShare["loop"], porFloorSigmas, 165680},
		{"atomic under the tight limits", porTightShare["atomic"], porTightSigmas, 43588},
	} {
		if floor := porFloorAt(300000, c.share, c.sigmas, false); c.got < floor {
			t.Errorf("%s: %d of 300000 models, the floor is %d", c.name, c.got, floor)
		}
	}
	if floor := porFloorAt(300000, 0.3, 5, false); floor > int(0.95*90000) {
		t.Errorf("a floor of %d is closer than 5%% to the mean of 90000", floor)
	}
}

func TestPORFloorValues(t *testing.T) {
	for _, c := range []struct {
		models int
		share  float64
		sigmas float64
		short  bool
		want   int
	}{
		{3000, 0.14, 6, false, 305}, // mean 420, deviation 19.0
		{3000, 0.14, 5, false, 324},
		{375, 0.14, 6, true, 0}, // half the share: mean 26, deviation 4.9: too small to say anything
		{375, 0.30, 5, true, 21},
		{8000, 0.05, 6, false, 283}, // mean 400, deviation 19.5
		{10, 0.5, 6, false, 0},
		{0, 0.5, 6, true, 0},
	} {
		if got := porFloorAt(c.models, c.share, c.sigmas, c.short); got != c.want {
			t.Errorf("porFloorAt(%d, %v, %v, %v) = %d, want %d", c.models, c.share, c.sigmas, c.short, got, c.want)
		}
	}
}

// A run whose generator reaches the share does not fail the floor by chance, at
// any size a test uses (the default sizes and an eighth of them, go test
// -short): the exact probability of a count below the floor is negligible for
// every share of the tables.
func TestPORFloorsAreNotReachedByChance(t *testing.T) {
	for _, tab := range []struct {
		name   string
		shares map[string]float64
		sigmas float64
	}{{"porReducedShare", porReducedShare, porFloorSigmas}, {"porTightShare", porTightShare, porTightSigmas}} {
		if len(tab.shares) == 0 {
			t.Fatalf("%s is empty", tab.name)
		}
		for gen, s := range tab.shares {
			for _, c := range []struct {
				models int
				short  bool
			}{{3000, false}, {8000, false}, {300000, false}, {1000, true}, {375, true}} {
				floor := porFloorAt(c.models, s, tab.sigmas, c.short)
				if p := binomialLowerTail(c.models, s, floor); p > 5e-7 {
					t.Errorf("%s[%s] = %v, %d models (short %v): P(count < floor %d) = %.2g, more than 5e-7", tab.name, gen, s, c.models, c.short, floor, p)
				}
			}
		}
	}
}
