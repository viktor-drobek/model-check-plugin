int total;

active proctype worker()
{
  do
  :: total = total + 1
  :: total > 3 -> break
  od
}
