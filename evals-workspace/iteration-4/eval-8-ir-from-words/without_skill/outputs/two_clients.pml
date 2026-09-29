#define N 2

mtype = { go };

/* Clients may enqueue requests while the server is occupied. */
chan request = [N] of { byte };
chan permission[N] = [0] of { mtype };
chan release = [0] of { byte };

bool working[N];
byte owner = N; /* N means that the server is free. */

proctype Client(byte id)
{
    do
    :: request!id;
       permission[id]?go;
       working[id] = true;
#ifdef CRASH_AFTER_GRANT
       /* Extended scenario: client 0 stops during work without releasing. */
       if
       :: id == 0 -> break
       :: else -> skip
       fi;
#endif
       skip; /* Abstract finite work. */
       working[id] = false;
       release!id
    od
}

proctype Server()
{
    byte id;
    do
    :: request?id;
       assert(owner == N);
       owner = id;
       permission[id]!go;
       release?eval(id);
       owner = N
    od
}

init {
    atomic {
        run Client(0);
        run Client(1);
        run Server()
    }
}

ltl mutual_exclusion { [] !(working[0] && working[1]) }
