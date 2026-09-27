/* probe: arrays of channels
 * claim: the reference's §3 says this is INSIDE the subset
 */
chan c[2] = [1] of { byte };
active proctype P() { c[0]!1 }
