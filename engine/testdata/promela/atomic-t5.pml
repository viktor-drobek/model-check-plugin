byte x;
active proctype A(){ do :: atomic { x < 3 -> x++ } :: atomic { x > 0 -> x-- } od }
