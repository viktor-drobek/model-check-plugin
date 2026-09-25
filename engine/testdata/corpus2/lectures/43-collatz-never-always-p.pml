/*
 * Source: books-md/lect01-lect09.md, lines 6542-6542 (lecture slide: the same model with the never claim generated for ![]p)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Dropped slide annotation: «> gcc > ./pan -a ...»
 */
int x = 100;
#define p (x > 0 && x <= 100)

active proctype A() {
    do
    :: x%2 -> x = 3*x + 1
    od
}

active proctype B() {
    do
    :: !x%2 -> x = x/2
    od
}

never {    /* ![]p */
T0_init:
    if
    :: (! ((p))) -> goto accept_all
    :: (1) -> goto T0_init
    fi;
accept_all:
    skip
}
