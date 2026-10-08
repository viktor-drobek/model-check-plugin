/* POR fixture (performance plan, step 6, process creation): `run` in a loop.
   The engine pre-instantiates a bounded pool of D; the run that finds the pool
   empty stops the search ("process budget exhausted"), which answers
   inconclusive. The reduction must stop at the same place, not before and not
   never. */
proctype D() { skip }
init { do :: run D() od }
