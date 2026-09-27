/* probe: two if options declaring one name
 * claim: the reference's §3 names says this is OUTSIDE the subset
 */
active proctype P() { if :: byte n; n = 1 :: byte n; n = 2 fi }
