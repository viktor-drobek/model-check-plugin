/* probe: len, empty, full, nempty, nfull
 * claim: the reference's §1 Channels says this is INSIDE the subset
 */
chan c = [2] of { byte };
active proctype P() { byte n; n = len(c);
  if :: empty(c) -> skip :: full(c) -> skip :: nempty(c) -> skip :: nfull(c) -> skip fi }
