/* probe: ltl name { … } block
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
bit x;
active proctype P() { do :: x = 1 - x od }
ltl p { []<> x }
