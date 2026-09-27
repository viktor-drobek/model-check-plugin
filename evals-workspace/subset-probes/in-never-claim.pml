/* probe: never { … }
 * claim: the reference's §1 Properties says this is INSIDE the subset
 */
bit x;
active proctype P() { do :: x = 1 - x od }
never { do :: !x -> skip od }
