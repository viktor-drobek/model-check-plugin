/* probe: provided (e)
 * claim: the reference's §3 says this is INSIDE the subset
 */
byte turn;
active proctype P() provided (turn == 0) { turn = 1 }
