| benchmark | native | baseline-qjs | v8go script-per-call | v8go precompiled | native / v8go script |
|---|---|---|---|---|---|
| GenerateSingle person.firstName | 6.14 us | 34.04 us | 1.07 us | 0.82 us | 5.7x |
| GenerateSingle internet.email | 36.42 us | 110.83 us | 3.40 us | 2.65 us | 10.7x |
| GenerateSingle location.streetAddress | 47.75 us | 139.51 us | 3.84 us | 3.08 us | 12.4x |
| GenerateSingle lorem.sentence | 112.52 us | 205.39 us | 2.05 us | 1.78 us | 54.8x |
| GenerateSingle string.uuid | 48.98 us | 114.64 us | 3.07 us | 2.81 us | 16.0x |
| GenerateSingle number.int | 1.47 us | 20.18 us | 1.15 us | 0.93 us | 1.3x |
| Row20 percall | 476.2 us | 1476.2 us | 39.5 us | 33.4 us | 12.1x |
| Row20 batched | 460.7 us | 842.0 us | 35.1 us | 30.0 us | 13.1x |
| NewEngine | 312 us | 23423 us | 4749 us | 4744 us | 0.1x |
