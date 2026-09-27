/* probe: chan c = [N] of { t }; send and receive
 * claim: the reference's §1 Channels says this is INSIDE the subset
 */
chan c = [2] of { byte };
active proctype S() { c!1 }
active proctype R() { byte x; c?x }
