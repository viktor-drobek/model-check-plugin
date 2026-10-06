/* POR fixture: S sends once, which fills the channel (capacity 1), and then
   chooses between sending again and doing nothing. The second send is disabled
   while the channel is full and R's receive enables it. If S takes it, S waits
   for x == 5, which nobody sets: a deadlock that exists only behind the send
   that R's receive enables. S must not be expanded alone while the channel is
   full, or the deadlock is lost. */
byte x;
chan c = [1] of { byte };

active proctype S() {
  c!1;
  if
  :: c!2 -> (x == 5)
  :: skip
  fi
}

active proctype R() {
  c?_
}
