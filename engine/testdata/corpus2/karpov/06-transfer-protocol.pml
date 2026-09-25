/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 4665-4671 (§7.1, W.C.Lynch data-transfer protocol: the transfer process and init)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 4665, 4671.
 * Printed line numbers 1-38 removed; blank lines kept where a number carried no code.
 */
mtype = { ack, nak, err, next, accept };

proctype transfer(chan in,out,CHin,CHout)
{  byte o, i;    /* переменные для выходных и входных сообщений */
   in ? next(o); /* по каналу in ждем сообщение для передачи */

 do   /* ожидаем один из трех типов сообщений nack, ack, err */
 :: CHin ? nak(i) ->
            out ! accept(i);
            CHout ! ack(o)

 :: CHin ? ack(i) ->
            out ! accept(i);
            in ? next(o);
            CHout ! ack(o)

 ::     CHin ? err(i) ->
            CHout ! nak(o)
 od
 }

init
 { chan AtoB = [1] of { mtype, byte };
   chan BtoA = [1] of { mtype, byte };

   chan Ain  = [2] of { mtype, byte };
   chan Bin  = [2] of { mtype, byte };

   chan Aout = [2] of { mtype, byte };
   chan Bout = [2] of { mtype, byte };

   atomic {
            run transfer(Ain, Aout, AtoB, BtoA);
            run transfer(Bin, Bout, BtoA, AtoB)
          };

   AtoB!err(0)
 }
