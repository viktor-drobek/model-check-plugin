/* A block-scoped redeclaration of a local. SPIN 6.5.2 refuses the file
 * ("redeclaration of 'n'"); the frontend used to accept it and collapse the
 * two variables into one, so the model that was checked was not the model
 * that was written. Probe and family in steps/g5-addendum2-confirmation.md.
 *
 * The scope rule SPIN follows, and this frontend now follows: a declaration
 * is an error when the name is visible where it stands — the same scope, or
 * one still open around it. Two *sibling* blocks may reuse a name, and do
 * share one variable; that shape is the scenario's counterpart.
 */
active proctype P()
{
	byte n;
	n = 1;
	{	byte n;
		n = 2
	}
}
