package lsp

// LSPInput is the typed input for LSPTool.
type LSPInput struct {
	Operation string `json:"operation"`
	FilePath  string `json:"filePath"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}
