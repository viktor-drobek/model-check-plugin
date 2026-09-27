/* probe: proctype parameters, _pid
 * claim: the reference's §1 Processes says this is INSIDE the subset
 */
proctype P(byte x) { byte me = _pid; me = x }
init { run P(3) }
