byte x;

active proctype P() {
  { x = 1; x = 2 } unless { x == 1 -> x = 3 };
  assert(x == 3)
}
