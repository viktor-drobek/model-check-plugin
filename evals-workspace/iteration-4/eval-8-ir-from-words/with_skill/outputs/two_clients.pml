// Учебная абстракция словесного протокола: синхронные каналы, без отказов.
chan request = [0] of { byte };
chan grant0 = [0] of { bit };
chan grant1 = [0] of { bit };
chan release = [0] of { byte };

bool waiting0 = false;
bool waiting1 = false;
bool accepted0 = false;
bool accepted1 = false;
bool working0 = false;
bool working1 = false;

active proctype Client0() {
  do
  :: waiting0 = true;
     request!0;
     accepted0 = true;
     grant0?1;
     accepted0 = false;
     waiting0 = false;
     working0 = true;
     working0 = false;
     release!0
  od
}

active proctype Client1() {
  do
  :: waiting1 = true;
     request!1;
     accepted1 = true;
     grant1?1;
     accepted1 = false;
     waiting1 = false;
     working1 = true;
     working1 = false;
     release!1
  od
}

active proctype Server() {
  do
  :: request?0;
     grant0!1;
     release?0
  :: request?1;
     grant1!1;
     release?1
  od
}
