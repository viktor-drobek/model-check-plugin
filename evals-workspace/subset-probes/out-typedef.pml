/* probe: typedef
 * claim: the reference's §3 says this is INSIDE the subset
 */
typedef Pair { byte a; byte b };
Pair p;
active proctype P() { p.a = 1 }
