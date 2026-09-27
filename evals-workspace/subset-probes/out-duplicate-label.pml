/* probe: duplicate label in one proctype
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
active proctype P() {
L: skip;
L: skip }
