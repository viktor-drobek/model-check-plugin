/* probe: block-scoped redeclaration of a local
 * claim: the reference's §3 known defect says this is OUTSIDE the subset
 */
active proctype P() { byte n; n = 1; { byte n; n = 2 } }
