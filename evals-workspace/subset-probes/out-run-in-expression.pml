/* probe: run as the whole right-hand side (pid = run P())
 * claim: the reference's §3 says this is INSIDE the subset
 */
proctype Q() { skip }
byte n;
init { n = run Q() }
