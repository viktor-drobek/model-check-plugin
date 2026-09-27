/* probe: c?x with a constant (pattern match)
 * claim: the reference's §1 Channels says this is INSIDE the subset
 */
mtype = { m };
chan c = [2] of { mtype, byte };
active proctype R() { byte x; c?m,x }
