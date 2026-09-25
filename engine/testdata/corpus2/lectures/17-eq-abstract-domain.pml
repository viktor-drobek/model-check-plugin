/*
 * Source: books-md/lect01-lect09.md, lines 3339-3339 (lecture slide: helper definitions, a single abstract data domain)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * The identical listing is repeated at markdown lines 3339, 3357 and 3375.
 */
bool z;
mtype {M1,M2} m = M1;

proctype EQ(byte x, byte y) {
    if
    :: (x == y) -> z = true
    :: else -> z = false
    fi
}
