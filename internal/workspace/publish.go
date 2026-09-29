package workspace

// WriteLocalJSON uses the workspace's atomic, flushed replacement primitive.
// Callers must first validate containment of path in their host-local state.
func WriteLocalJSON(path string, value any) error {
	return atomicWriteJSON(path, value)
}

// ReplaceFile publishes a flushed temporary sibling with the same replacement
// primitive used for local configuration on Windows and Unix.
func ReplaceFile(temp, destination string) error {
	return atomicRename(temp, destination)
}
