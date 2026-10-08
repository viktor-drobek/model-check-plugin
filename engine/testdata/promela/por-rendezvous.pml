/* POR fixture (performance plan, step 6): a rendezvous channel. A handshake is
   one step of two processes, so the reduction is refused for the model and says
   so. */
chan r = [0] of { byte };
active proctype S() { r!1 }
active proctype T() { byte v; r?v }
