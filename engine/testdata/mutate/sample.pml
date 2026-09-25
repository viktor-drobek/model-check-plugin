/* A small model that carries every construct the K3 mutation operators look
 * for (plan 14 §8.1): an atomic and a d_step wrapper, a buffered channel, a
 * bare guard, an end label, an assert, a relational operator, constants and
 * multi-option if/do. It is a fixture for the operator tests, not a model of
 * anything.
 */
#define LIMIT 3

chan c = [2] of { byte };
byte n;

active proctype worker()
{
end:	do
	:: (n < LIMIT) -> atomic { n = n + 1; c!n }
	:: (n > 0) -> d_step { n = n - 1 }
	:: c?n -> assert(n <= LIMIT)
	od
}
