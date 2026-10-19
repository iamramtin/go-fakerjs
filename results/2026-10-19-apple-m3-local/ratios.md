| benchmark | native | baseline-qjs | v8go script-per-call | v8go precompiled | native / v8go script |
|---|---|---|---|---|---|
| GenerateSingle person.firstName | 7.81 us | 40.04 us | 1.44 us | 1.14 us | 5.4x |
| GenerateSingle internet.email | 46.79 us | 136.01 us | 4.27 us | 3.70 us | 11.0x |
| GenerateSingle location.streetAddress | 61.30 us | 174.53 us | 4.61 us | 4.65 us | 13.3x |
| GenerateSingle lorem.sentence | 144.75 us | 252.95 us | 2.69 us | 2.31 us | 53.9x |
| GenerateSingle string.uuid | 64.21 us | 137.83 us | 3.96 us | 3.62 us | 16.2x |
| GenerateSingle number.int | 1.90 us | 24.55 us | 1.41 us | 1.25 us | 1.4x |
| Row20 percall | 616.8 us | 1797.7 us | 51.5 us | 43.9 us | 12.0x |
| Row20 batched | 581.6 us | 1090.8 us | 46.1 us | 39.2 us | 12.6x |
| NewEngine | 370 us | 32102 us | 5575 us | 5581 us | 0.1x |
