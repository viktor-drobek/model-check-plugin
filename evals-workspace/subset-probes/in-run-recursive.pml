/* probe: recursive run (G5)
 * claim: the reference's §1 Processes says this is INSIDE the subset
 */
proctype R(byte d) { if :: d > 0 -> run R(d - 1) :: else -> skip fi }
init { run R(2) }
