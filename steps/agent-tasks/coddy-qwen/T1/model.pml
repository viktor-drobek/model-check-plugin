byte credits;

active proctype issuer()
{
  do
  :: credits++
  od
}

active proctype spender()
{
  do
  :: credits > 0 -> credits--
  od
}
