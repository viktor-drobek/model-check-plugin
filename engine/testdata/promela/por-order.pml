/* POR fixture: two senders share a channel. The receiver asserts that the
   first message is 1, which fails when S2's message arrives first. The two
   sends depend on each other (the order in the buffer), and must be explored
   in both orders. */
byte a;
chan c = [2] of { byte };

active proctype S1() { c!1 }
active proctype S2() { c!2 }
active proctype R() {
  c?a;
  assert(a == 1);
  c?_
}
