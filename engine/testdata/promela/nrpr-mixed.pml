/* active and run together, _nr_pr read. A starts W (the youngest, pid 2)
 * and waits until it is alone; the older B can leave only after W has gone.
 * pan: no error, 15 states stored.
 */
byte x;
proctype W() { x = x + 1 }
active proctype A() { run W(); (_nr_pr == 1); x = 10 }
active proctype B() { skip }
