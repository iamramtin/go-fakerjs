//go:build race

package stress

// raceFactor scales the time and memory budgets of the resource-bomb test: the race detector makes
// wasm2go code roughly 6x slower on these inputs and adds shadow memory to RSS. Outcomes (the error class and
// engine usability) are asserted identically; only the budgets change.
const raceFactor = 3
