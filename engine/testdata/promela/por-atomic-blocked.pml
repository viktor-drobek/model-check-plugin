/* POR fixture (performance plan, step 6, atomic sequences): P and R each
   enter an atomic sequence whose second statement blocks for ever (z is never
   7). A process blocked inside its sequence loses exclusive control but the
   exclusive byte stays set, so the two stuck states (P then R, R then P) differ
   only in that byte. The deadlock must be found. */
byte z = 0;
active proctype P() { atomic { skip; z == 7 } }
active proctype R() { atomic { skip; z == 7 } }
