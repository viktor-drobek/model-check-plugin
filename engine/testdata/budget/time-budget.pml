byte i; bit a;
active proctype P(){ atomic { do :: i < 40 -> i++; if :: a=0 :: a=1 fi :: else -> break od } }
active proctype Q(){ skip }
