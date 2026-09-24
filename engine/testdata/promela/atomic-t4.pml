byte x, y;
active proctype A(){ atomic { x = 1; y == 1; x = 2 } }
active proctype B(){ y = 1 }
