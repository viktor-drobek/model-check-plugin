/*
 * Source: books-md/lect01-lect09.md, lines 4050-4050 (lecture slide: data-type abstraction with mtype plus an inline definition)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Dropped slide annotation: «Не загромождаем модель»
 */
mtype {X1, X2, X3};

proctype f(mtype x){
    do
    :: x == X1 -> printf('Case 1\n'); incr(x);
    :: x == X3 -> break;
    :: x == X2 -> printf('Case 2\n'); incr(x);
    od
}

init{
    mtype x;
    if
    :: x = X1
    :: x = X2
    :: x = X3
    fi;
    run f(x)
}

inline incr(x){
    if
    :: x += 1;
    :: skip
    fi
}
