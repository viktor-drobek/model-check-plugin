/* probe: redeclaration while the enclosing scope is open
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
active proctype P() { byte n; n = 1; { byte n; n = 2 } }
