/* probe: #define NAME body
 * claim: the reference's §1 Preprocessor says this is INSIDE the subset
 */
#define LIMIT 3
byte n;
active proctype P() { n = LIMIT }
