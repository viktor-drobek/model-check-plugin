/*
 * Source: books-md/lect01-lect09.md, lines 2925-2925 (lecture slide: Promela has no block scope for local variables)
 * Second corpus for plan 14 §2.3. The listing was OCR-joined onto one line;
 * line breaks are reconstructed from Promela syntax and nothing else is changed.
 * Dropped slide annotation: «Ошибка, повторное объявление y Переменная z всё ещё видна!»
 */
active proctype main() {
    int x, y;
    {
        int y, z;
        x++;
        y++;
        z++
    };
    printf('y=%d, z=%d\n', y, z);
}
