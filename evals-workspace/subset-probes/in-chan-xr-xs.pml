/* probe: xr c / xs c as hints
 * claim: the reference's §1 Channels says this is INSIDE the subset
 */
chan c = [2] of { byte };
active proctype P() { xr c; xs c; c!1 }
