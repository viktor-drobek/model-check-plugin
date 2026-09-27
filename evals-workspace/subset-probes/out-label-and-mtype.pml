/* probe: label colliding with an mtype constant
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
mtype = { m };
active proctype P() {
m: skip }
