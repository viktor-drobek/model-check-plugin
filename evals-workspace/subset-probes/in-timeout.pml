/* probe: timeout
 * claim: the reference's §1 Statements says this is INSIDE the subset
 */
chan c = [0] of { byte };
active proctype P() { byte x; do :: c?x :: timeout -> break od }
