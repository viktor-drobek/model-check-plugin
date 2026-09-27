/* probe: c_expr
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
active proctype P() { if :: c_expr { 1 } -> skip fi }
