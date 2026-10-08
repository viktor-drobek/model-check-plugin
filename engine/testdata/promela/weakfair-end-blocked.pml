/* A second process that waits for ever at an `end` label while the first one finishes. The property [] (a -> <> b) holds on every
 * maximal run (b is set right after a), and weak fairness only removes runs, so it must be verified under weak fairness whenever it
 * is verified without. Release 0.1.1 and 0.2.0 reported `violated` under --fairness weak: the lasso was null steps of the fairness
 * construction only, while the claim sat in an accepting state it could not leave (issue 1 of the public repository). */
bool a = false;
bool b = false;
chan c = [1] of { bit };
active proctype P() { a = true; b = true }
active proctype Q() { end: c ? 1 }
