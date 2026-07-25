package file

// FileReadInput is the typed input for FileReadTool.
type FileReadInput struct {
	FilePath string  `json:"file_path"`
	Offset   float64 `json:"offset,omitempty"`
	Limit    float64 `json:"limit,omitempty"`
	Pages    string  `json:"pages,omitempty"`
}

// FileWriteInput is the typed input for FileWriteTool.
type FileWriteInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// FileEditInput is the typed input for FileEditTool.
type FileEditInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}
