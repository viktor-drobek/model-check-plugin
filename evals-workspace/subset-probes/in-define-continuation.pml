/* probe: #define with a \ continuation
 * claim: the reference's §1 Preprocessor says this is INSIDE the subset
 */
#define BOTH(a, b) a = 1; \
                   b = 2
byte x; byte y;
active proctype P() { BOTH(x, y) }
