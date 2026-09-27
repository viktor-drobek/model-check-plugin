/* probe: #define NAME(args) body
 * claim: the reference's §1 Preprocessor says this is INSIDE the subset
 */
#define INC(v) v = v + 1
byte n;
active proctype P() { INC(n) }
