/* probe: #ifdef/#ifndef/#if/#elif/#else/#endif, defined(), #undef
 * claim: the reference's §1 Preprocessor says this is INSIDE the subset
 */
#define A 1
#ifdef A
byte n;
#endif
#ifndef B
byte m;
#endif
#if defined(A)
byte k;
#elif 1
byte j;
#else
byte i;
#endif
#undef A
active proctype P() { n = 1; m = 2; k = 3 }
