/* CH12/leader (Dolev, Klawe & Rodeh leader election) rewritten for N = 3
 * without the constructs outside the engine's Promela subset: the channel
 * array q[N] becomes q0..q2, the channel parameters of `node` are folded
 * into three copies of the proctype, and the `run` loop of init is
 * unrolled. The instance numbers are those of the original for N = 3,
 * I = 3: (N+I-proc)%N+1 gives 3, 2, 1 for proc = 1, 2, 3. Everything
 * else is the original text. The property is CH12/leader.ltl:
 * <>[]oneLeader with #define oneLeader (nr_leaders == 1). */

#define N	3	/* nr of processes */
#define I	3	/* node given the smallest number    */
#define L	10	/* size of buffer  (>= 2*N) */

mtype = { one, two, winner };
chan q0 = [L] of { mtype, byte };
chan q1 = [L] of { mtype, byte };
chan q2 = [L] of { mtype, byte };

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
NODE(node3, q2, q0, 1)

init {
	atomic {
		run node1();
		run node2();
		run node3()
	}
}
