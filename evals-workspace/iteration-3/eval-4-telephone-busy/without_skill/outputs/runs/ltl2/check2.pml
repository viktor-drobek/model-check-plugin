#include "version1.pml"
#define sw_busy   (switch[1]@Busy)
#define sub_busy  (subscriber[0]@Busy)

/* P4: whenever the switch is in Busy, the subscriber is off-hook in its own Busy
       (i.e. the on-hook branch is always available to release the switch)      */
ltl p4_busy_implies_sub_busy { [] (sw_busy -> sub_busy) }

/* P5: response form of the same question */
ltl p5_busy_leads_to_free    { [] (sw_busy -> <> !sw_busy) }

/* P6: the switch returns to Idle infinitely often (progress of the whole call) */
ltl p6_idle_inf_often        { [] <> (switch[1]@Idle) }
