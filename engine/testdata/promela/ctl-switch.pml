/* A three-state switch, used by features/g5-ctl-v1.feature for the CTL
 * cases. The reachable graph is exactly x = 0, 1, 2 with the process always
 * at the loop head, so every CTL answer can be checked by hand:
 *   0 -> 1, 1 -> 2, 1 -> 0, 2 -> 0
 * EF (x == 2) holds (0,1,2 is a witness), EG (x < 3) holds on the loop
 * 0,1,0,…, AF (x == 2) fails on that same loop, and x is never 7 — which
 * makes AG ((x == 7) -> (x == 8)) vacuously true.
 */
byte x;

active proctype A()
{
end:	do
	:: x == 0 -> x = 1
	:: x == 1 -> x = 2
	:: x == 1 -> x = 0
	:: x == 2 -> x = 0
	od
}
