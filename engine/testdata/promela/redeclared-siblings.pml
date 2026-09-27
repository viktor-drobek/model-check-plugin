/* The legal counterpart of redeclared-local.pml: two sibling blocks declare
 * one name, the first scope having closed before the second opens. SPIN
 * accepts it, keeps ONE variable and re-initialises it at each declaration
 * — six states, transitions "n = 0", "n = 1", "n = 0", "n = 2" — and so
 * does this frontend.
 */
active proctype P()
{
	{	byte n;
		n = 1
	};
	{	byte n;
		n = 2
	}
}
