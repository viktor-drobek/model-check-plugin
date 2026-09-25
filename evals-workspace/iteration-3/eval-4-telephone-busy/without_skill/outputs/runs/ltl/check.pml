/* Wrapper: original model is included unchanged, only observers are added. */
#include "version1.pml"

#define sw_busy   (switch[1]@Busy)      /* switch (MSC) sits in state Busy   */
#define sub_busy  (subscriber[0]@Busy)  /* subscriber sits in state Busy     */

/* P1: the switch cannot hang forever in Busy */
ltl p1_switch_not_stuck_busy { ! (<> [] sw_busy) }

/* P2: the subscriber cannot hang forever in Busy (off-hook forever) */
ltl p2_sub_not_stuck_busy    { ! (<> [] sub_busy) }

/* P3: sanity - is the switch's Busy state reachable at all?
       expect this to FAIL, i.e. a counterexample = a run that enters Busy */
ltl p3_busy_unreachable      { [] !sw_busy }
