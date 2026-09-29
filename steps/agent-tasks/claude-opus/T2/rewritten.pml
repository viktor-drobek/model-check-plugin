int total;

active proctype worker()
{
  do
  :: total <= 3 -> total = total + 1
  :: else -> break
  od
}
