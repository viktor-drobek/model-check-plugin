/* probe: label colliding with a global
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
byte g;
active proctype P() {
g: skip }
