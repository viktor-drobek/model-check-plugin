/* probe: atomic { … }, d_step { … }
 * claim: the reference's §1 Atomicity says this is INSIDE the subset
 */
byte n;
active proctype P() { atomic { n = 1; n = 2 }; d_step { n = 3; n = 4 } }
