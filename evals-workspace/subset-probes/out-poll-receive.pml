/* probe: c?[…] poll
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
chan c = [1] of { byte };
active proctype P() { byte x; if :: c?[x] -> skip fi }
