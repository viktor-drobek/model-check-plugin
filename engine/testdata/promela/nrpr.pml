/* Dynamic process creation inside a loop, with _nr_pr (G5). init starts two
 * workers one after the other; each worker checks how many processes are
 * live. The point of the model is that `run` stands inside a `do` option,
 * so its proctype gets a pool of instances rather than one, and that the
 * live-process table gives _nr_pr and pan's pid order.
 */
byte started;

proctype worker(byte k)
{
	assert(_nr_pr > 1)
}

init {
	do
	:: started < 2 -> started++; run worker(started)
	:: else -> break
	od
}
