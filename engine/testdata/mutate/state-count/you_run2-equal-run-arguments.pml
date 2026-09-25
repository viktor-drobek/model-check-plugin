/*
 * K3 secondary finding: the verdicts agree, the state counts do not
 * (plan 14 §8.1: "расхождение счётчиков — повод для разбора, а не
 * автоматический провал").
 * Model:    CH3/you_run2.pml
 * Mutation: off-by-one at line 9, column 15 — "0" -> "1", so that init
 *           starts two instances of you_run with the SAME argument.
 * Engine:   verified, 14 states stored (mcd check --sweep --unlimited)
 * pan:      verified, 12 states stored (spin -a -o1 -o2 -o3;
 *           gcc -O2 -DNOREDUCE; ./pan -c0), state vector 28 byte
 * The original (arguments 0 and 1) agrees at 14 = 14, so the difference
 * appears only when the two instances carry equal parameter values: pan
 * then merges two pairs of states that the engine keeps apart.
 * Reported, not fixed: the engine belongs to step G5.
 */
proctype you_run(byte x)
{
	printf("x = %d, pid = %d\n", x, _pid)
}

#if 1
	/* leaving pids implicit */
init {
	run you_run(1);
	run you_run(1)
}

#else
	/* storing pids in local vars */
init {	pid p0, p1;

	p0 = run you_run(0);
	p1 = run you_run(1);
	printf("pids: %d and %d\n", p0, p1)
}

#endif
