/*
 * Source: books-md/lect01-lect09.md, lines 4793-4805 (lecture slide: alternating bit protocol with a lossy channel and timeout)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 4793, 4805.
 */
mtype = {msg, ack};
chan to_s = [1] of {mtype, bit};
chan to_r = [1] of {mtype, bit};
chan from_s = [1] of {mtype, bit};
chan from_r = [1] of {mtype, bit};

active proctype sender() {
    bit a;
    do
    :: from_s!msg,a ->
        if
        :: to_s?ack,eval(a) -> a = 1 - a
        :: timeout
        fi
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

active proctype {
    mtype m;
    bit a;
    do
    :: from_s?m,a ->
        if
        :: skip
        fi
    :: from_r?m,a -> to_s!m,a
    od
}
