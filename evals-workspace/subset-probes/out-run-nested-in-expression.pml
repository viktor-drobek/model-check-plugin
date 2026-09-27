/* probe: run nested inside a larger expression
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
proctype Q() { skip }
byte n;
init { n = 1 + run Q() }
