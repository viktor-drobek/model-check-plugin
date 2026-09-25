/*
 * Source: books-md/lect01-lect09.md, lines 3177-3189 (lecture slide: alternating bit protocol over two buffered channels, eval())
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 3177, 3189.
 * The same listing appears again at markdown lines 3860, 3872.
 * The slide itself stops after the inner od; the closing brace of receiver() is missing in the book and is left missing here.
 * Dropped slide annotation: «Считываем новое сообщение Сохраняем сообщение Игнорируем сообщение»
 */
mtype = {msg, ack};
chan s_r = [2] of {mtype, bit};
chan r_s = [2] of {mtype, bit};

active proctype sender() {
    bit seqno;
    do
    :: s_r!msg,seqno ->
        if
        :: r_s?ack,eval(seqno) -> seqno = 1 - seqno;
        :: r_s?ack,eval(1-seqno)
        fi
    od
}

active proctype receiver() {
    bit expect, seqno;
    do
    :: s_r?msg,seqno ->
        r_s!ack,seqno;
        if
        :: seqno == expect; expect = 1 - expect
        ::else
        fi
    od
