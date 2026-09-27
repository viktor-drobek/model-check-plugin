/* probe: pc_value(pid)
 * claim: the reference's §3 (plan §5.2 lists it as v1) says this is OUTSIDE the subset
 */
byte n;
active proctype P() { skip }
active proctype Q() { n = pc_value(0) }
