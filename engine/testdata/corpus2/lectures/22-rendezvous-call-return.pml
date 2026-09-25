/*
 * Source: books-md/lect01-lect09.md, lines 4306-4306 (lecture slide: typical mistake, modelling a call with rendezvous channels)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
chan f1 = [0] of bit;
chan f2 = [0] of bit;

active proctype caller() {
    do
    :: if
       ::f1!m;
       :: else -> break
       fi;
       ...
    od
}

active proctype callee() {
    msg m;
    do
    :: f1?m -> f2!m
    od
}
