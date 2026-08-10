package review

// diskFreeBytes reports the number of bytes free for use by the current
// user on the filesystem containing path, using only build-tagged
// standard-library platform calls (see diskspace_windows.go,
// diskspace_linux.go, diskspace_other.go). It never shells out and never
// reads or writes any file content; ok is false when the platform-specific
// call fails or is unsupported on this OS, in which case callers must treat
// availability as unknown rather than assuming either sufficiency or
// insufficiency.
//
// This is a read-only diagnostic used only to annotate manifest analysis
// with "would this destination likely have room" information; it never
// influences whether an action is generated or executed.
func diskFreeBytes(path string) (bytes int64, ok bool) {
	return platformDiskFreeBytes(path)
}
