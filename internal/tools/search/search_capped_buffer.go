// Package search provides the capped buffer used by ripgrep execution.
// to bound stdout capture and protect against runaway match sets.
package search

import (
	"bytes"
	"io"
)

// cappedBuffer is a bytes.Buffer that silently drops writes once it has
// reached `cap` bytes. The dropped flag is exposed so callers can surface
// a "results truncated" hint when relevant.
type cappedBuffer struct {
	bytes.Buffer
	cap     int
	dropped bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	remaining := c.cap - c.Buffer.Len()
	if remaining <= 0 {
		c.dropped = true
		return len(p), nil
	}
	if len(p) > remaining {
		c.dropped = true
		_, err := c.Buffer.Write(p[:remaining])
		return len(p), err
	}
	return c.Buffer.Write(p)
}

// ReadFrom overrides the promoted bytes.Buffer.ReadFrom method. os/exec copies
// child stdout with io.Copy, which prefers ReaderFrom when available; without
// this override it bypasses Write and silently defeats the cap.
func (c *cappedBuffer) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			total += int64(n)
			if _, writeErr := c.Write(buf[:n]); writeErr != nil {
				return total, writeErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}
