byte x = 1;

active proctype P() {
  { x = 2 } unless { x == 1 -> x = 3 };
  assert(x == 3)
}
