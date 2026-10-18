// Package fakerjs runs the real Faker.js (version [FakerVersion]) from pure Go, with no cgo and no Wasm runtime.
//
// Faker.js and QuickJS-NG are compiled to WebAssembly, pre-initialised with Wizer so the Faker data is
// already loaded, and translated to Go source by wasm2go. An [Engine] is one private instance of that
// translated program.
//
// Status: experimental. The API may change before v1.0.0; see FINDINGS.md for the evidence behind it.
package fakerjs

// FakerVersion is the Faker.js release embedded in the snapshot. tools.versions is its source of truth;
// `make check-versions` fails if the two differ.
const FakerVersion = "10.6.0"
