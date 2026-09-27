/* probe: local shadowing a global
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
byte n;
active proctype P() { byte n; n = 1 }
