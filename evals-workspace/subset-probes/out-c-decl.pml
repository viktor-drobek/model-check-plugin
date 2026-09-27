/* probe: c_decl
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
c_decl { extern int q; }
active proctype P() { skip }
