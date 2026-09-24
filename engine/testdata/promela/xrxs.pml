/* G1: xr/xs are accepted and stored in the IR as hints (they matter only for POR, vNext) */
mtype = { msg, ack };
chan q = [2] of { mtype, byte };
chan r = [2] of { mtype };

active proctype S()
{	byte s = 1;
	xs q;
	xr r;
	do
	:: q!msg(s) -> r?ack
	od
}

active proctype R()
{	byte v;
	xs r;
	xr q;
	do
	:: q?msg(v) -> r!ack
	od
}
