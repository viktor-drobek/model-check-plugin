/* probe: c!e1,e2 and c!e1(e2) forms
 * claim: the reference's §1 Channels says this is INSIDE the subset
 */
mtype = { m };
chan c = [2] of { mtype, byte };
active proctype S() { c!m,1; c!m(2) }
