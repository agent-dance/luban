package tools

func withRuntimeFileLock(path string, fn func() error) error {
	unlock, err := lockRuntimeFile(path)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

func withRuntimeFileLockResult(path string, fn func() (any, error)) (any, error) {
	unlock, err := lockRuntimeFile(path)
	if err != nil {
		return nil, err
	}
	defer unlock()
	return fn()
}
