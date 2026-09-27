/* probe: sibling blocks declaring one name with different types
 * claim: the reference's §3 divergences says this is OUTSIDE the subset
 */
active proctype P() { { int y; y = 1 }; { byte y; y = 2 } }
