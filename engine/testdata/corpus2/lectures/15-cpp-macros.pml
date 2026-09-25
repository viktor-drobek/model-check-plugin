/*
 * Source: books-md/lect01-lect09.md, lines 2956-2956 (lecture slide: the C preprocessor in Promela (#define, #ifdef, #if 0))
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * The dropped text is leading slide prose, glued in front of the code by OCR.
 * Dropped slide annotation: «- константы (альтернатива: spin -DMAXQ=2 …) - макросы - условный код»
 */
chan q=[MAXQ] of {mtype,chan};
#define RESET(a)\
atomic {a[0] = 0; a[1] = 0}
#define LOSSY 1
...
#ifdef LOSSY
active proctype D()
#endif
#if 0
COMMENTS
#endif
