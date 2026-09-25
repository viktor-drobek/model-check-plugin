/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 4691-4733 (§7.2, the PAR protocol: users, sender, receiver and lossy channels)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 4691, 4711, 4719, 4725, 4733.
 * Printed line numbers removed. The book's own numbering is wrong in the channel listing (it runs 37, 38, 39, 30, 31, ... and reuses 31); the code is left exactly as printed.
 * The misspelled keyword qoto (for goto) is kept as printed.
 */
/* описание протокола PAR */
mtype = { d0, d1, ACK }        /* возможные сообщения в каналах */
chan c1, c2, c3, c4, c5, c6 = [0] of { mtype }     /* 6 каналов */
bool s0, s1, r0, r1 = false;
proctype UserA() {
  q0: c1! d0; s0 = true;  s1 = false;
  q1: c1! d1; s0 = false; s1 = true;
  q2: c1! d0; s0 = true; s1 = false; goto q1;
}

proctype UserB(){
  q0: c4? d0; r0 = true; r1 = false;
  q1: c4? d1; r0 = false; r1 = true;
  q2: c4? d0; r0 = true; r1 = false; goto q1;
}
proctype Sender() { mtype msg;
 q0: c1? d0;
 q1: c2! d0;
 q2: if
     :: c6? ACK -> qoto q3
     :: true  -> goto q1 /* передатчик не дождался подтверждения */
     fi;
 q3: c1? d1;
 q4: c2! d1;
 q5: if
     :: c6? ACK -> qoto q0
     :: true  -> goto q4 /* передатчик не дождался подтверждения */
     fi;
}

proctype Receiver() {
/* процесс Receiver строится полностью аналогично процессу Sender */
proctype CHANNEL1() { mtype msg;
  q0: c2? msg;
  if
  :: msg == d0; ->
     if
     :: c3! d0
     :: true -> skip
     fi
  :: msg == d1; ->
     if
     :: c3! d1
     :: true -> skip
     fi
  fi;
  goto q0
}
proctype CHANNEL2() { mtype msg;
 /* процесс Channel2 строится полностью аналогично процессу Channel1 */
}
