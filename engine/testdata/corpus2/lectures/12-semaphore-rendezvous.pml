/*
 * Source: books-md/lect01-lect09.md, lines 2776-2776 (lecture slide: a semaphore modelled with a rendezvous channel)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
mtype = { P, V };
chan sema = [0] of { mtype };

active proctype semaphore() {
L:   sema!P ->
     sema?V;
     goto L
}

active [5] proctype user() {
L:   /*non-critical*/
     sema?P ->
     /*critical*/
     sema!V;
     goto L;
}
