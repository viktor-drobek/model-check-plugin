/*
 * Source: books-md/lect01-lect09.md, lines 2515-2515 (lecture slide: the same consumer written with if..fi plus goto)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
active proctype consumer() {
again:
    if
    :: (turn == C) ->
        printf('Consume\n');
        turn = P
    fi;
    goto again
}
