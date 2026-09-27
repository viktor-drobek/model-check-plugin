/* probe: run inside an expression
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
proctype Q() { skip }
byte n;
init { n = run Q() }
