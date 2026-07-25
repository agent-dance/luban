package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/cli"
)

func TestPrepareInputTransportReadsOnlyImplicitPrintPrompt(t *testing.T) {
	tests := []struct {
		name          string
		opts          cli.Options
		stdinTerminal bool
		wantPrint     bool
		wantArgs      []string
		wantRead      bool
	}{
		{name: "implicit print reads pipe", wantPrint: true, wantArgs: []string{"piped prompt"}, wantRead: true},
		{name: "positional query wins", opts: cli.Options{Args: []string{"position query"}}, wantPrint: true, wantArgs: []string{"position query"}},
		{name: "explicit print does not consume pipe", opts: cli.Options{Print: true}, wantPrint: true},
		{name: "SDK owns pipe", opts: cli.Options{SDK: true}, wantPrint: false},
		{name: "terminal remains interactive", stdinTerminal: true, wantPrint: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := &countingPromptReader{Reader: strings.NewReader("piped prompt")}
			opts := test.opts
			if err := prepareInputTransport(&opts, test.stdinTerminal, reader); err != nil {
				t.Fatal(err)
			}
			if opts.Print != test.wantPrint {
				t.Fatalf("Print = %v, want %v", opts.Print, test.wantPrint)
			}
			if strings.Join(opts.Args, "\x00") != strings.Join(test.wantArgs, "\x00") {
				t.Fatalf("Args = %#v, want %#v", opts.Args, test.wantArgs)
			}
			if test.wantRead && reader.reads == 0 {
				t.Fatal("stdin was not read")
			}
			if !test.wantRead && reader.reads != 0 {
				t.Fatalf("stdin reads = %d, want zero", reader.reads)
			}
		})
	}
}

func TestPrepareInputTransportPropagatesPipeReadError(t *testing.T) {
	wantErr := errors.New("read failed")
	opts := cli.Options{}
	if err := prepareInputTransport(&opts, false, errorPromptReader{err: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

type countingPromptReader struct {
	*strings.Reader
	reads int
}

func (r *countingPromptReader) Read(buffer []byte) (int, error) {
	r.reads++
	return r.Reader.Read(buffer)
}

type errorPromptReader struct{ err error }

func (r errorPromptReader) Read([]byte) (int, error) { return 0, r.err }
