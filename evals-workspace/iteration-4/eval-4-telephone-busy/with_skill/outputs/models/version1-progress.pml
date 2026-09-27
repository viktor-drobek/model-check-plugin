/* DERIVED MODEL — not a corpus file.
 *
 * Source: "Promela - examples/CH14/version1" (Holzmann, Fig 13.4 / 13.5),
 * sha256 acdfcacad083c29ff47cba18de2fb84ec784e9e064e0b64c435497e07121ec3b.
 *
 * The one and only change with respect to the source: the subscriber's control
 * location `Idle` is renamed to `progress_Idle` (and the `goto` that targets it),
 * so that the label carries the `progress` prefix.  Nothing else — no statement,
 * no channel, no branch — is touched.
 *
 * Meaning of the change (it is an assumption, and it is reported as one):
 * "progress" is defined as the subscriber being back on the hook, i.e. the phone
 * having left `Busy` and being able to start a new call.  A non-progress cycle is
 * then exactly an infinite run of the system in which the phone never returns to
 * `Idle` — the hang the user asks about.
 */

mtype = { offhook, digits, onhook };

chan tpc = [0] of { mtype };

active proctype subscriber()
{
progress_Idle:	tpc!offhook;

Busy:	if
	:: tpc!digits -> goto Busy
	:: tpc!onhook -> goto progress_Idle
	fi
}

active proctype switch()	/* outgoing calls only */
{
Idle:
	if
	:: tpc?offhook -> printf("MSC: dialtone\n"); goto Dial
	fi;
Dial:
	if
	:: tpc?digits -> printf("MSC: notone\n"); goto Wait
	:: tpc?onhook -> printf("MSC: notone\n"); goto Idle
	fi;
Wait:
	if
	:: printf("MSC: ringtone\n") -> goto Connect;
	:: printf("MSC: busytone\n") -> goto Busy
	fi;
Connect:
	if
	:: printf("MSC: busytone\n") -> goto Busy
	:: printf("MSC: notone\n")   -> goto Busy	/* no remote hang up */
	fi;
Busy:
	tpc?onhook -> printf("MSC: notone\n"); goto Idle
}
