/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 5317-5323 (§8.4, Needham-Schroeder: the intruder process)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 5317, 5323.
 * The book gives only the idea of the program, with ... standing for elided alternatives; nonceВ and I_knows_nonceВ use the Cyrillic letter В and are kept as printed.
 */
active proctype Intruder( ) {
    bool I_knows_nonceA = false, I_knows_nonceB = false;
    /* вначале злоумышленник не знает секретов А и В */
    do          /* цикл подслушивания и посылок сообщений */
    :: network [0] ? (msg, data)-> /* если подслушал любое сообщение */
       if
       :: intercepted = data; /* либо запоминает перехваченные данные */
       :: skip; /* либо нет */
       fi;
       if
       :: data.key == keyI -> /* если в принятом сообщении - свой ключ */
          if /* то извлекает чужой nonce, если он есть */
          :: data.info1==nonceA||data.info2==nonceA->I_knows_nonceA= true;
          :: data.info1==nonceВ||data.info2==nonceВ->I_knows_nonceВ= true;
          :: else -> skip;
          fi
       :: else -> skip;        /* если в принятом сообщении - не свой ключ */
       fi;
       /* посылает свое сообщение собирая его из возможных кусков */
    ::if          /* произвольно выбирает тип сообщения */
      :: msg = msg1;
      :: ... /* недетерминированно делает то же для msg2 и msg3 */
      fi;
      if   /* случайно выбирает получателя, выдавая себя за А, В или I*/
      :: data.sender=agentA -> data.receiver = agentB; data.key = pkeyB; .../* или выдает себя за B, или за I, и выбирает получателей
      fi;
      if  /* собирает свое сообщение для посылки */
      :: data=intercepted;  /* перехваченные данные шлет без изменения */
      :: if                 /* либо собирает содержимое сообщения для А */
         :: data.info1 = agentA;
         :: I_knows_nonceA -> data.info1 = nonceA;
         ...  /* то же для В */
         fi
         ... /* то же для data.info2  */
      :: if /* собирает содержимое сообщения для В */
         :: ...
         :: ...
         fi
      fi;
      network [1]! (msg, recpt, data);
    od;
