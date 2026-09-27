/* probe: c?? random/sorted receive
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
chan c = [1] of { byte };
active proctype P() { byte x; c??x }
