/* probe: remote variable reference P[i]:var
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
active proctype P() { byte v; v = 1 }
active proctype Q() { byte w; w = P[0]:v }
