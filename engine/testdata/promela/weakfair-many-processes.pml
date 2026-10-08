/* More than 125 processes under weak fairness. Every null step of the lasso
 * (no process moves, the fairness counter advances to the next copy) is kept in
 * a small signed number of the explorer's frames; with 126 or more processes
 * the copy index overflowed it, and the trace showed the null steps of the
 * higher copies as stutter steps of the system. The verdict was right, the
 * lasso was not. */
bool f;

active [130] proctype P()
{
	f
}
