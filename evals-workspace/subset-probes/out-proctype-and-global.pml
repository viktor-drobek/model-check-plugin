/* probe: proctype name colliding with a global
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
byte P;
active proctype P() { skip }
