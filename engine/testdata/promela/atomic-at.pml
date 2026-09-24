byte x, y;
active proctype A() { atomic { x = 1; y == 1; x = 2 }; x = 3 }
active proctype B() { y = 1 }
