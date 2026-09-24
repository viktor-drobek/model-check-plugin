/* G1: blocking inside d_step is a model error (SPIN: "block in d_step seq") */
byte x;

active proctype A()
{
	d_step { x = 1; x == 2; x = 3 }
}

active proctype B()
{
	x = 2
}
