/* probe: end, progress, accept label prefixes
 * claim: the reference's §1 Control says this is INSIDE the subset
 */
bit x;
active proctype P() { do :: x = 1 - x od }
active proctype Q() {
progress_p: x = 1;
accept_a: x = 0;
end_e: skip }
