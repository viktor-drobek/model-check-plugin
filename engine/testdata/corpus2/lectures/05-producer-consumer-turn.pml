/*
 * Source: books-md/lect01-lect09.md, lines 2495-2495 (lecture slide: producer/consumer on a turn variable)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
mtype = { P, C };
mtype turn = P;

active proctype producer() {
    do
    :: (turn == P) ->
        printf('Produce\n');
        turn = C
    od
}

active proctype consumer() {
    do
    :: (turn == C) ->
        printf('Consume\n');
        turn = P
    od
}
