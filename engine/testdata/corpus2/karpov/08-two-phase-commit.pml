/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 4763-4769 (§7.3, the two-phase commit protocol: manager, N active processes, assert)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 4763, 4769.
 * Printed line numbers 1-54 removed; blank lines kept where a number carried no code.
 */
#define N 6                         /*  число активных процессов  */
mtype = {START, COMMIT, ABORT, NODECISION}; /*символьные константы*/
chan Mchan = [0] of {mtype},    /* канал от менеджера к процессам */
     Pchan = [0] of {mtype};    /* канал от процессов к менеджеру */
mtype GlobalDecision = NODECISION;        /* коллективное решение */

proctype Manager(){ byte count = 0;
     /* count считает число взаимодействий с активными процессами */
 do  /* ШАГ1: запрос ко всем активным процессам принять решение*/
 :: (count < N) -> Mchan ! START; count ++
 :: (count == N) -> break
 od;

     /* ШАГ2: прием решений от всех активных процессов */
GlobalDecision = COMMIT; /* сначала полагаем общее решение - COMMIT
Это решение останется, если ни один из процессов не пришлет ABORT */
 mtype vote;    /* vote - переменная для приема решений процессов */
 do
 :: (count < 2*N ) -> Pchan ? vote; count ++;
   if
   :: (vote == ABORT) -> GlobalDecision = ABORT
   :: (vote == COMMIT) -> skip
     /* если хотя бы один процесс прислал ABORT, то решение ABORT */
   fi
 :: (count == 2*N) -> break
 od;
 /* ШАГ3: рассылка общего решения всем активным процессам */
 do
 :: (count < 3*N) -> Mchan ! GlobalDecision; count ++
 :: (count == 3*N) -> break
 od
}

proctype Active_process (byte id) /*активный процесс с номером id */
{ mtype decision = NODECISION;
      /*  сначала у процесса решения нет  */
 Mchan ? START - >  /* стартует, когда получен запуск от менеджера */
  if
    /*  случайно выбираем свое решение и отправляем его менеджеру */
    /* потом ожидаем общее решение от менеджера  */
  :: Pchan ! ABORT;  Mchan ? decision; decision = ABORT
  :: Pchan ! COMMIT; Mchan ? decision
  fi; assert (decision == GlobalDecision)
  /* если мы решили ABORT, общее решение должно быть только ABORT */
}
          /* Оставшаяся часть программы инициирует процессы */
init      /* инициатор запускает N активных процессов и менеджер */
{ byte count = 0;
  do
  :: (count  < N) -> run Active_process (count); count++
  :: (count == N) -> break
  od;
  run Manager();
}
