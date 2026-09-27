/* probe: run with more arguments than parameters
 * claim: the reference's §3 divergences says this is OUTSIDE the subset
 */
proctype P(byte x) { x = 1 }
init { run P(1, 2) }
