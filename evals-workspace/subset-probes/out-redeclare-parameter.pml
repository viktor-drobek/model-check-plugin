/* probe: local shadowing a proctype parameter
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
proctype P(byte n) { byte n; n = 1 }
init { run P(1) }
