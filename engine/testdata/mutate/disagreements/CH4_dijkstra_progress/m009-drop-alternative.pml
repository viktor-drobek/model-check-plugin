/*
 * K3 class (iii) DISAGREEMENT (plan 14 §8.1, §9 K3).
 * Model:    CH4/dijkstra_progress.pml
 * Mutation: drop-alternative at line 9, column 2
 *           ":: (count == 1) ->\nprogress:\tsema!p; count = 0" -> (removed)
 * Check non-progress (pan -l):   engine violated, pan verified
 * Reported, not fixed: the engine belongs to step G5.
 */
mtype { p, v };

chan sema = [0] of { mtype };

active proctype Dijkstra()
{	byte count = 1;

end:	do
	:: (count == 0) ->
		sema?v; count = 1
	od	
}

active [3] proctype user()
{	do
	:: sema?p;	   /* enter */
critical:  skip;	   /* leave */
	   sema!v;
	od
}
