/* A goto to a label no statement carries. SPIN reports "undefined label";
 * so does this frontend, with the label, the file and the line. Dropping
 * the jump silently would check a different model than the one written.
 */
byte x;

active proctype A()
{
	x = 1;
	goto Nowhere
}
