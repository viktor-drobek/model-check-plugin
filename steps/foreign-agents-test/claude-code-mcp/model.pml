bool busy;
byte inside;

active [2] proctype client()
{
  do
  :: !busy ->
       busy = true;
       inside++;
       assert(inside == 1);
       inside--;
       busy = false
  od
}
