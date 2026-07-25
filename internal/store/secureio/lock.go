package secureio

// WithRuntimeFileLock runs fn while holding the platform file lock at path.
func WithRuntimeFileLock(path string, fn func() error) error {
	unlock, err := lockRuntimeFile(path)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// WithRuntimeFileLockResult runs fn while holding the platform file lock at
// path and returns its value.
func WithRuntimeFileLockResult(path string, fn func() (any, error)) (any, error) {
	unlock, err := lockRuntimeFile(path)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return fn()
}
