/* probe: proctype, active proctype, active [N] proctype, init
 * claim: the reference's §1 Processes says this is INSIDE the subset
 */
proctype P() { skip }
active proctype A() { skip }
active [2] proctype B() { skip }
init { run P() }
