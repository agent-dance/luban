package worktree

type cappedBuffer struct {
	buf     []byte
	cap     int
	dropped bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	written := len(p)
	remaining := b.cap - len(b.buf)
	if remaining <= 0 {
		b.dropped = b.dropped || written > 0
		return written, nil
	}
	if len(p) > remaining {
		b.buf = append(b.buf, p[:remaining]...)
		b.dropped = true
		return written, nil
	}
	b.buf = append(b.buf, p...)
	return written, nil
}

func (b *cappedBuffer) String() string { return string(b.buf) }
