/* probe: c_track
 * claim: the reference's §3 says this is OUTSIDE the subset
 */
c_code { int q; }
c_track "&q" "sizeof(int)"
active proctype P() { skip }
