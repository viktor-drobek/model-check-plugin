/* probe: sibling blocks declaring one name (the scope has closed)
 * claim: the reference's §3 names says this is INSIDE the subset
 */
active proctype P() { { byte n; n = 1 }; { byte n; n = 2 } }
