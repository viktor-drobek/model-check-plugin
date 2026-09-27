/* probe: remote label reference P@label inside a never claim
 * claim: the reference's §1 Expressions says this is INSIDE the subset
 */
bit x;
active proctype P() {
L0: x = 1;
L1: x = 0 }
never { do :: P@L0 -> skip od }
