/* probe: mtype = { a, b }
 * claim: the reference's §1 mtype says this is INSIDE the subset
 */
mtype = { red, green };
active proctype P() { mtype m; m = red; m = green }
