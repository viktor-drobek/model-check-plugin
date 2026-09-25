#include "version2.pml"
#define sw2_busy (switch_ss7[1]@Busy)
ltl v2_switch_not_stuck_busy { ! (<> [] sw2_busy) }
ltl v2_idle_inf_often        { [] <> (switch_ss7[1]@Idle) }
