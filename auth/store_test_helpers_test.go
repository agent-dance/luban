package auth

func newStoreAt(dir string) *Store {
	return &Store{dir: dir}
}
