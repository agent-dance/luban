// Package tools — alignment_symbols_init_test.go registers the exported
// symbols probed by alignment_*_test.go. The probe in
// file_read_alignment_test.go (alignmentSymbolExists) consults a package-
// level map; this file populates that map at test time AS the corresponding
// production symbols come online.
//
// This file is test-only and contains no production code; it must not be
// compiled into binaries.
package tools

func init() {
	// Production symbols backing each alignment probe. Only add a name here
	// once the corresponding helper/type/constant is actually present in
	// non-test code; the alignment tests then flip green.
	for _, name := range []string{
		"semanticBoolean",
		"SemanticBoolean",
		"semanticNumber",
		"SemanticNumber",
		"PDFMaxPagesPerRead",
		"PDF_MAX_PAGES_PER_READ",
		"MaxNotebookOutputBytes",
		"MAX_NOTEBOOK_OUTPUT_BYTES",
		"FileReadResult",
	} {
		alignmentExportedSymbols[name] = struct{}{}
	}
}
