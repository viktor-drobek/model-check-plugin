/* D1 (hand claim): blocked system, claim alternates accepting / non-accepting on true. */
bit a;
active proctype P() { a == 1 }
never {
accept_S0: if :: (true) -> goto S1 fi;
S1: if :: (true) -> goto accept_S0 fi;
}
