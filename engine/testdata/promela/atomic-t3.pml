byte x, y;
active proctype A(){ atomic { x = 1; if :: y = 1 :: y = 2 fi; x = 2 } }
active proctype B(){ x = 9 }
