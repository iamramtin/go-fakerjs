
#### Cold start (Apple M5, median of 10)

| benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| NewEngine/baseline-qjs | 23,422,997 | 21,788,178 | 1,748 |
| NewEngine/native | 311,624 | 6,056,667 | 509 |
| NewEngine/v8go-precompiled | 4,743,776 | 837,144 | 35 |
| NewEngine/v8go-script-per-call | 4,749,015 | 836,854 | 31 |
| NewFromSnapshot | 337,848 | 6,056,448 | 503 |

#### GenerateSingle (Apple M5, ns/op, median of 10)

| method | native | baseline-qjs | v8go script per call | v8go precompiled | native / v8go script |
|---|---|---|---|---|---|
| person.firstName | 6,138 | 34,040 | 1,071 | 816 | 5.7x |
| internet.email | 36,423 | 110,830 | 3,402 | 2,646 | 10.7x |
| location.streetAddress | 47,754 | 139,513 | 3,837 | 3,076 | 12.4x |
| lorem.sentence | 112,520 | 205,390 | 2,052 | 1,779 | 54.8x |
| string.uuid | 48,984 | 114,642 | 3,067 | 2,806 | 16.0x |
| number.int | 1,474 | 20,178 | 1,154 | 926 | 1.3x |

#### GenerateN amortised ns per value (Apple M5, median of 10)

| method | n | native | baseline-qjs | qjs / native |
|---|---|---|---|---|
| person.firstName | 1 | 6,152 | 35,134 | 5.7x |
| person.firstName | 10 | 5,971 | 13,908 | 2.3x |
| person.firstName | 100 | 5,936 | 11,556 | 1.9x |
| person.firstName | 1000 | 5,924 | 11,256 | 1.9x |
| internet.email | 1 | 36,773 | 111,456 | 3.0x |
| internet.email | 10 | 36,418 | 78,097 | 2.1x |
| internet.email | 100 | 36,415 | 75,798 | 2.1x |
| internet.email | 1000 | 36,258 | 73,972 | 2.0x |
| location.streetAddress | 1 | 48,467 | 138,394 | 2.9x |
| location.streetAddress | 10 | 48,148 | 104,087 | 2.2x |
| location.streetAddress | 100 | 47,950 | 101,880 | 2.1x |
| location.streetAddress | 1000 | 47,989 | 98,740 | 2.1x |
| lorem.sentence | 1 | 112,430 | 204,002 | 1.8x |
| lorem.sentence | 10 | 112,210 | 175,074 | 1.6x |
| lorem.sentence | 100 | 111,787 | 175,004 | 1.6x |
| lorem.sentence | 1000 | 111,934 | 172,575 | 1.5x |
| string.uuid | 1 | 49,446 | 114,168 | 2.3x |
| string.uuid | 10 | 49,194 | 94,932 | 1.9x |
| string.uuid | 100 | 49,179 | 91,904 | 1.9x |
| string.uuid | 1000 | 49,050 | 91,422 | 1.9x |
| number.int | 1 | 1,483 | 20,222 | 13.6x |
| number.int | 10 | 1,318 | 4,744 | 3.6x |
| number.int | 100 | 1,298 | 3,048 | 2.3x |
| number.int | 1000 | 1,295 | 2,860 | 2.2x |

#### Row20 (Apple M5): one Generate per cell (percall) vs GenerateN per column (batched)

| engine | mode | rows/s | ns/row | B/op | allocs/op |
|---|---|---|---|---|---|
| native | percall | 2,100 | 476,156 | 4,340 | 45 |
| native | batched | 2,170 | 460,726 | 4,176 | 25 |
| baseline-qjs | percall | 677 | 1,476,210 | 9,974,016 | 2,217 |
| baseline-qjs | batched | 1,188 | 841,988 | 17,750 | 23 |
| v8go-script-per-call | percall | 25,339 | 39,466 | 2,641 | 85 |
| v8go-script-per-call | batched | 28,500 | 35,088 | 2,967 | 85 |
| v8go-precompiled | percall | 29,984 | 33,351 | 1,777 | 45 |
| v8go-precompiled | batched | 33,367 | 29,970 | 2,103 | 45 |

#### Row20 with k engines on k goroutines (Apple M5, 10 cores, aggregate rows/s, median of 10)

| engine | mode | engines | rows/s | speed-up vs 1 engine |
|---|---|---|---|---|
| native | percall | 1 | 2,128 | 1.00x |
| native | percall | 2 | 3,959 | 1.86x |
| native | percall | 4 | 5,653 | 2.66x |
| native | percall | 8 | 8,013 | 3.77x |
| native | percall | 10 | 9,652 | 4.54x |
| native | batched | 1 | 2,212 | 1.00x |
| native | batched | 2 | 4,083 | 1.85x |
| native | batched | 4 | 6,136 | 2.77x |
| native | batched | 8 | 8,696 | 3.93x |
| native | batched | 10 | 8,918 | 4.03x |
| baseline-qjs | percall | 1 | 680 | 1.00x |
| baseline-qjs | percall | 2 | 691 | 1.02x |
| baseline-qjs | percall | 4 | 704 | 1.04x |
| baseline-qjs | percall | 8 | 730 | 1.07x |
| baseline-qjs | percall | 10 | 727 | 1.07x |
| baseline-qjs | batched | 1 | 1,192 | 1.00x |
| baseline-qjs | batched | 2 | 1,842 | 1.55x |
| baseline-qjs | batched | 4 | 3,238 | 2.72x |
| baseline-qjs | batched | 8 | 4,100 | 3.44x |
| baseline-qjs | batched | 10 | 3,733 | 3.13x |
| v8go-script-per-call | percall | 1 | 25,630 | 1.00x |
| v8go-script-per-call | percall | 2 | 45,650 | 1.78x |
| v8go-script-per-call | percall | 4 | 85,902 | 3.35x |
| v8go-script-per-call | percall | 8 | 107,114 | 4.18x |
| v8go-script-per-call | percall | 10 | 111,128 | 4.34x |
| v8go-script-per-call | batched | 1 | 28,666 | 1.00x |
| v8go-script-per-call | batched | 2 | 54,587 | 1.90x |
| v8go-script-per-call | batched | 4 | 93,371 | 3.26x |
| v8go-script-per-call | batched | 8 | 117,280 | 4.09x |
| v8go-script-per-call | batched | 10 | 122,448 | 4.27x |
| v8go-precompiled | percall | 1 | 30,022 | 1.00x |
| v8go-precompiled | percall | 2 | 57,606 | 1.92x |
| v8go-precompiled | percall | 4 | 102,482 | 3.41x |
| v8go-precompiled | percall | 8 | 131,054 | 4.37x |
| v8go-precompiled | percall | 10 | 134,194 | 4.47x |
| v8go-precompiled | batched | 1 | 33,624 | 1.00x |
| v8go-precompiled | batched | 2 | 65,042 | 1.93x |
| v8go-precompiled | batched | 4 | 114,092 | 3.39x |
| v8go-precompiled | batched | 8 | 145,682 | 4.33x |
| v8go-precompiled | batched | 10 | 149,154 | 4.44x |

#### Row20 through the public Pool, native (Apple M5, aggregate rows/s, median of 10)

| mode | pool | rows/s | speed-up vs pool of 1 |
|---|---|---|---|
| percall | 1 | 2,111 | 1.00x |
| percall | 2 | 3,941 | 1.87x |
| percall | 4 | 6,846 | 3.24x |
| percall | 8 | 8,605 | 4.08x |
| batched | 1 | 2,218 | 1.00x |
| batched | 2 | 4,153 | 1.87x |
| batched | 4 | 7,342 | 3.31x |
| batched | 8 | 9,662 | 4.36x |
