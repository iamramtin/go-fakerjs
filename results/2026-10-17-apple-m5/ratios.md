| benchmark | native | baseline-qjs | v8go script-per-call | v8go precompiled | native / v8go script |
|---|---|---|---|---|---|
| GenerateSingle person.firstName | 6.18 us | 34.55 us | 1.10 us | 0.83 us | 5.6x |
| GenerateSingle internet.email | 36.84 us | 110.90 us | 3.21 us | 3.10 us | 11.5x |
| GenerateSingle location.streetAddress | 48.41 us | 139.04 us | 3.65 us | 3.11 us | 13.3x |
| GenerateSingle lorem.sentence | 112.40 us | 206.31 us | 2.05 us | 1.82 us | 54.7x |
| GenerateSingle string.uuid | 49.38 us | 114.51 us | 3.09 us | 2.83 us | 16.0x |
| GenerateSingle number.int | 1.48 us | 20.21 us | 1.14 us | 0.96 us | 1.3x |
| Row20 percall | 478.1 us | 1513.9 us | 39.0 us | 34.0 us | 12.3x |
| Row20 batched | 452.8 us | 835.3 us | 35.4 us | 29.9 us | 12.8x |
| NewEngine | 299 us | 22881 us | 4757 us | 4803 us | 0.1x |
