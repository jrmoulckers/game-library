//go:build !windows && !linux

package review

// platformDiskFreeBytes has no standard-library implementation on this
// platform. Rather than shell out to an external command or add a
// third-party dependency, this build reports availability as unknown; the
// dashboard's manifest analysis represents that explicitly (see
// DestinationSpace.AvailableBytesKnown) instead of guessing.
func platformDiskFreeBytes(path string) (int64, bool) {
	return 0, false
}
