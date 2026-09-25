/*
 * Source: books-md/lect01-lect09.md, lines 4719-4719 (lecture slide: leader election on a ring, channel arrays and run in atomic)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * The identical listing is repeated at markdown line 4856.
 * The body of node() is elided with ... on the slide itself.
 */
#define N   5  /* nr of processes (use 5 for demos)*/
#define I   3  /* node given the smallest number   */
#define L   10 /* size of buffer  (>= 2*N) */

/* file ex.8 */

mtype = { one, two, winner };
chan q[N] = [L] of { mtype, byte};
byte nr_leaders = 0;

proctype node (chan in, out; byte mynumber) {
    bit Active = 1, know_winner = 0;
    byte nr, maximum = mynumber, neighbourR;
    ...
}

init {
    byte proc;
    atomic {
        proc = 1;
        do
        :: proc <= N ->
            run node (q[proc-1], q[proc%N], (N+I-proc)%N+1);
            proc++
        :: proc > N -> break
        od
    }
}
