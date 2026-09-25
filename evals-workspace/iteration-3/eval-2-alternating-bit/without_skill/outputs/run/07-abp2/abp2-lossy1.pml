mtype = { msg, ack };

chan	to_sndr = [2] of { mtype, bit };
chan	to_rcvr = [2] of { mtype, bit };

active proctype Sender()
{	bool seq_out, seq_in;

	/* obtain first message */
	do
	:: to_rcvr!msg(seq_out) ->
		to_sndr?ack(seq_in);
		if
		:: seq_in == seq_out ->
			/* obtain new message */
			seq_out = 1 - seq_out;
		:: else
		fi
	od
}

active proctype Receiver()
{	bool seq_in;

	do
	:: to_rcvr?msg(seq_in) ->
		to_sndr!ack(seq_in)
	:: timeout ->	/* recover from msg loss */
		to_sndr!ack(seq_in)
	od
}

/* same single-loss daemon as abp-lossy1.pml */
byte budget = 1;
active proctype Loss()
{	mtype m; bit b;
	do
	:: atomic { (budget > 0 && len(to_rcvr) > 0) -> to_rcvr?m(b); budget-- }
	:: atomic { (budget > 0 && len(to_sndr) > 0) -> to_sndr?m(b); budget-- }
	:: budget == 0 -> break
	od
}
