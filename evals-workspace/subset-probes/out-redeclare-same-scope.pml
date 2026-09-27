/* probe: redeclaration in the same scope
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
active proctype P() { byte n; byte n; n = 1 }
