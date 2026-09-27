/* probe: one-dimensional array of fixed length
 * claim: the reference's §1 Types says this is INSIDE the subset
 */
byte a[3];
active proctype P() { a[1] = 2 }
