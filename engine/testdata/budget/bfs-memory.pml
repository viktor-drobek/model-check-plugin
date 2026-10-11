int x;
active proctype P(){ atomic { do :: break :: x++ od } }
active proctype Q(){ skip }
