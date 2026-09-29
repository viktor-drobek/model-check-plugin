bool want[2];
byte served;

active [2] proctype client()
{
  byte me = _pid;
  do
  :: want[me] = true;
     served = me;
     want[me] = false
  od
}
