// Package fakerjs runs Faker.js inside a Go program, so Go code generates the same fake data as Faker.js
// from the same seed, value for value.
//
// An [Engine] calls a Faker.js method by name, such as "person.fullName", and returns the result as the
// text JavaScript's String() gives. A non-zero seed and [WithRefDate] make its sequence reproducible. A
// [Pool] runs engines in parallel, each seeded with [DeriveSeed].
//
// The embedded release is Faker.js [FakerVersion] with the en locale. Faker.js and the QuickJS-NG
// interpreter are compiled to WebAssembly and translated to Go source, so the package needs no cgo and no
// Wasm runtime, and imports only the standard library. The API may change before v1.0.0.
//
// Reference:
// https://fakerjs.dev/api/
package fakerjs

// FakerVersion is the Faker.js release embedded in the snapshot. tools.versions is its source of truth;
// `make check-versions` fails if the two differ.
const FakerVersion = "10.6.0"
