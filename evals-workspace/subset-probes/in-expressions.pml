/* probe: arithmetic, comparison, &&, ||, !, %
 * claim: the reference's §1 Expressions says this is INSIDE the subset
 */
byte n;
active proctype P() { n = (1 + 2) * 3 - 4;
  if :: n > 1 && n < 100 || !(n == 0) -> n = n % 5 :: else -> skip fi }
