byte x;
active proctype A(){ atomic { x = 1; x = 2; x = 3 } }
active proctype B(){ atomic { x = 4; x = 5 } }
