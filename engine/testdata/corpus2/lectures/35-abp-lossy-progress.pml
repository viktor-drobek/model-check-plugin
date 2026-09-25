/*
 * Source: books-md/lect01-lect09.md, lines 4837-4837 (lecture slide: the lossy channel refined with a progress label (abp_lossy2))
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 */
active proctype channel() {
    mtype m;
    bit a;
    do
    :: from_s?m,a ->
        if
        :: true -> to_r!m,a
        :: skip; progress: skip
        fi
    :: from_r?m,a -> to_s!m,a
    od
}

active proctype receiver() {
    bit a;
    do
    :: to_r?msg,eval(a) ->
        from_r!ack,a;
        progress: a = 1 - a
    od
}
