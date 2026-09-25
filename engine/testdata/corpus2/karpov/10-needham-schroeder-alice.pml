/*
 * Source: books-md/Karpov_U._Model_checking/Karpov_U._Model_checking.md, lines 5259-5297 (§8.4, Needham-Schroeder: constants, typedef, network channels and Alice)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Pieces joined from markdown lines 5259, 5265, 5273, 5283, 5291, 5297.
 * The book gives only the idea of the program; nonceА in the comment uses the Cyrillic letter А and the guard uses the typeset arrow ⇒, both kept as printed.
 */
mtype = { msg, /* тип сообщения */
alice, bob, intruder,    /* идентификаторы собеседников */
pkA, pkB, pkI,           /* публичные ключи собеседников */
nonceA, nonceB, nonceI,  /* случайные числа собеседников */
ok, err};

typedef mCrypt {mtype sender, receiver, key, info1, info2}

chan network[2]=[0] of { mtype, /*мнемоническое имя-номер сообщения */
mCrypt /*само сообщение - структура данных */ }

active proctype Alice( )  {
    mtype pkey, pnonce; /* публичный ключ и nonce партнера */
    mCrypt data;
    /* выбор собеседника: A может захотеть установить контакт с B или I */
    if
    :: partnerA=agentB; pkey=keyB /* msg от A будет направлено B */
    :: partnerA=agentI; pkey=keyI /* msg от A будет направлено I */
    fi
    /* посылка первого сообщения со своим nonce выбранному собеседнику */
    network [0]! (msg1, mCrypt {Alice, partnerA, pkey, nonceA, 0 } )
    /* прием nonce собеседника из второго сообщения, если оно зашифровано публичным ключом keyA и содержит ранее посланный nonceА */
    network [1]? (msg2, data);
    (data.key==keyA)&&(data.info1 == nonceA) ⇒ pnonce = data.info2;
    /* посылка принятого nonce выбранному ранее собеседнику */
    network [0]! (msg3, mCrypt {Alice, partnerA, pkey, 0, pnonce } );
    statusA = ok;  /* агент А завершился успешно */
}
