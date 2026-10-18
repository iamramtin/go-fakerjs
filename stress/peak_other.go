//go:build !unix

package stress

// PeakRSS is unavailable on this platform.
func PeakRSS() uint64 { return 0 }
