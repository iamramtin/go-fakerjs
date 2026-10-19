//go:build !race

package fakerjs

// raceEnabled reports whether the race detector is on. The detector makes long workloads too slow for the CI budget.
const raceEnabled = false
