package sdk

import (
	"io"
	"sync"
)

const sdkWriterQueueSize = 32

type sdkWriteRequest struct {
	line []byte
	ack  chan error
}

// sdkMessageWriter is the sole owner of the SDK output stream while Serve is
// running. Producers wait for an acknowledgement, which preserves causal
// ordering and applies bounded backpressure.
type sdkMessageWriter struct {
	out      io.Writer
	requests chan sdkWriteRequest
	done     chan struct{}
	onError  func()

	submitMu  sync.RWMutex
	closed    bool
	closeOnce sync.Once
	errMu     sync.RWMutex
	err       error
}

func newSDKMessageWriter(out io.Writer, onError func()) *sdkMessageWriter {
	w := &sdkMessageWriter{
		out:      out,
		requests: make(chan sdkWriteRequest, sdkWriterQueueSize),
		done:     make(chan struct{}),
		onError:  onError,
	}
	go w.run()
	return w
}

func (w *sdkMessageWriter) run() {
	defer close(w.done)
	for request := range w.requests {
		err := writeSDKLine(w.out, request.line)
		request.ack <- err
		if err != nil {
			w.setError(err)
			w.failPending(err)
			if w.onError != nil {
				w.onError()
			}
			return
		}
	}
}

func writeSDKLine(out io.Writer, line []byte) error {
	written, err := out.Write(line)
	if err != nil {
		return err
	}
	if written != len(line) {
		return io.ErrShortWrite
	}
	if flusher, ok := out.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func (w *sdkMessageWriter) submit(line []byte) error {
	request := sdkWriteRequest{line: line, ack: make(chan error, 1)}
	w.submitMu.RLock()
	if w.closed {
		w.submitMu.RUnlock()
		return io.ErrClosedPipe
	}
	select {
	case w.requests <- request:
		w.submitMu.RUnlock()
	case <-w.done:
		w.submitMu.RUnlock()
		return w.terminalError()
	}
	select {
	case err := <-request.ack:
		return err
	case <-w.done:
		select {
		case err := <-request.ack:
			return err
		default:
			return w.terminalError()
		}
	}
}

func (w *sdkMessageWriter) failPending(err error) {
	for {
		select {
		case request, ok := <-w.requests:
			if !ok {
				return
			}
			request.ack <- err
		default:
			return
		}
	}
}

func (w *sdkMessageWriter) terminalError() error {
	if err := w.Err(); err != nil {
		return err
	}
	return io.ErrClosedPipe
}

func (w *sdkMessageWriter) setError(err error) {
	w.errMu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.errMu.Unlock()
}

func (w *sdkMessageWriter) Err() error {
	w.errMu.RLock()
	defer w.errMu.RUnlock()
	return w.err
}

func (w *sdkMessageWriter) Close() error {
	w.closeOnce.Do(func() {
		w.submitMu.Lock()
		w.closed = true
		close(w.requests)
		w.submitMu.Unlock()
		<-w.done
	})
	return w.Err()
}
