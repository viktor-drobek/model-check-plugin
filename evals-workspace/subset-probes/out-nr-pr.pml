/* probe: _nr_pr
 * claim: the reference's §3 says this is INSIDE the subset
 */
byte n;
active proctype P() { n = _nr_pr }
