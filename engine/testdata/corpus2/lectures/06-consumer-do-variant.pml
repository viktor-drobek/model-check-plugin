/*
 * Source: books-md/lect01-lect09.md, lines 2509-2509 (lecture slide: the consumer process alone, do..od form)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
active proctype consumer() {
    do
    :: (turn == C) ->
        printf('Consume\n') ;
        turn = P
    od
}
