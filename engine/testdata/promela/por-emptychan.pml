/* POR fixture, the mirror of por-fullchan.pml: R chooses between receiving and
   doing nothing, and the receive is disabled while the channel is empty. S's
   send enables it. If R takes the receive it waits for x == 5, which nobody
   sets. R must not be expanded alone while the channel is empty. */
byte x;
chan c = [1] of { byte };

active proctype S() {
  c!1
}

active proctype R() {
  if
  :: c?_ -> (x == 5)
  :: skip
  fi
}
