/* probe: chan c = [0] of { t } (rendezvous)
 * claim: the reference's §1 Channels says this is INSIDE the subset
 */
chan c = [0] of { byte };
active proctype S() { c!1 }
active proctype R() { byte x; c?x }
