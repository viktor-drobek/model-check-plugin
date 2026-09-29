int total;

active proctype worker()
{
  do
  :: total = total + 1
  unless { total > 3 -> break }
  od
}
