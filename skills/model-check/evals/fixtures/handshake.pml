/* Reference model for eval 8 (plan 14 §12, assumption A7): the two-process
   handshake the eval describes in words, written by hand in the Promela
   subset. The eval compares the model the agent builds from the words with
   this one — same questions, same verdicts, not the same text.

   Words the eval gives: two clients share one server; a client sends a
   request, waits for the grant, works, then releases; the server grants to
   one client at a time and accepts the next request only after the release. */

mtype = { req, grant, rel };

chan to_server = [0] of { mtype, byte };
chan to_client = [0] of { mtype, byte };

byte working;      /* how many clients are in their working section */

active [2] proctype client()
{
  byte id = _pid;
end:
  do
  :: to_server ! req(id);
     to_client ? grant(id);
     working++;
     assert(working == 1);
     working--;
     to_server ! rel(id)
  od
}

active proctype server()
{
  mtype m; byte who;
  do
  :: to_server ? req(who);
     to_client ! grant(who);
     to_server ? rel(who)
  od
}
