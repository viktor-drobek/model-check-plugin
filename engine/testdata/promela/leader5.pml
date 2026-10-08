/* leader3.pml unrolled to N = 5 (performance plan, step 6): the same ring of
 * five nodes as CH12/leader, with the channels named (q0..q4) and the `run`
 * loop of init unrolled, which the engine can reduce and the original cannot
 * (its channels are `chan` parameters). 41692 states in full, the count of
 * CH9/leader.pml; with --por a little over a hundred. The property is
 * CH12/leader.ltl: <>[]oneLeader with #define oneLeader (nr_leaders == 1). */

#define N 5
#define I 3
#define L 10
mtype = { one, two, winner };
chan q0 = [L] of { mtype, byte };
chan q1 = [L] of { mtype, byte };
chan q2 = [L] of { mtype, byte };
chan q3 = [L] of { mtype, byte };
chan q4 = [L] of { mtype, byte };
byte nr_leaders = 0;
#define NODE(name, in, out, number)					\
proctype name()								\
{	bit Active = 1, know_winner = 0;				\
	byte nr, maximum = number, neighbourR;				\
	byte mynumber = number;						\
									\
	xr in;								\
	xs out;								\
									\
	printf("MSC: %d\n", mynumber);					\
	out!one(mynumber);						\
end:	do								\
	:: in?one(nr) ->						\
		if							\
		:: Active -> 						\
			if						\
			:: nr != maximum ->				\
				out!two(nr);				\
				neighbourR = nr				\
			:: else ->					\
				/* Raynal p.39:  max is greatest number */ \
				assert(nr == N);			\
				know_winner = 1;			\
				out!winner,nr;				\
			fi						\
		:: else ->						\
			out!one(nr)					\
		fi							\
									\
	:: in?two(nr) ->						\
		if							\
		:: Active -> 						\
			if						\
			:: neighbourR > nr && neighbourR > maximum ->	\
				maximum = neighbourR;			\
				out!one(neighbourR)			\
			:: else ->					\
				Active = 0				\
			fi						\
		:: else ->						\
			out!two(nr)					\
		fi							\
	:: in?winner,nr ->						\
		if							\
		:: nr != mynumber ->					\
			printf("MSC: LOST\n");				\
		:: else ->						\
			printf("MSC: LEADER\n");			\
			nr_leaders++;					\
			assert(nr_leaders == 1)				\
		fi;							\
		if							\
		:: know_winner						\
		:: else -> out!winner,nr				\
		fi;							\
		break							\
	od								\
}

NODE(node1, q0, q1, 3)
NODE(node2, q1, q2, 2)
NODE(node3, q2, q3, 1)
NODE(node4, q3, q4, 5)
NODE(node5, q4, q0, 4)
init {
	atomic {
		run node1();
		run node2();
		run node3();
		run node4();
		run node5()
	}
}
