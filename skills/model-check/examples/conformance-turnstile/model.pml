/* Abstract turnstile model: an accepted entry increments passages. */
byte passages = 0;

active proctype Turnstile() {
    atomic {
        passages = 1;
        assert(passages == 1)
    }
}
