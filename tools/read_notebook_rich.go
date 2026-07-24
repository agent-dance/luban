package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const notebookLargeOutputThreshold = 10000

type readNotebookJSON struct {
	Cells    []readNotebookCell `json:"cells"`
	Metadata struct {
		LanguageInfo struct {
			Name string `json:"name"`
		} `json:"language_info"`
	} `json:"metadata"`
}

type readNotebookCell struct {
	CellType       string               `json:"cell_type"`
	ID             string               `json:"id,omitempty"`
	Source         json.RawMessage      `json:"source"`
	ExecutionCount *int                 `json:"execution_count,omitempty"`
	Outputs        []readNotebookOutput `json:"outputs,omitempty"`
}

type readNotebookOutput struct {
	OutputType string                     `json:"output_type"`
	Text       json.RawMessage            `json:"text,omitempty"`
	Data       map[string]json.RawMessage `json:"data,omitempty"`
	EName      string                     `json:"ename,omitempty"`
	EValue     string                     `json:"evalue,omitempty"`
	Traceback  []string                   `json:"traceback,omitempty"`
}

type notebookReadCell struct {
	CellType       string               `json:"cellType"`
	CellID         string               `json:"cell_id"`
	Language       string               `json:"language,omitempty"`
	Source         string               `json:"source"`
	ExecutionCount *int                 `json:"execution_count,omitempty"`
	Outputs        []notebookReadOutput `json:"outputs,omitempty"`
}

type notebookReadOutput struct {
	OutputType string             `json:"output_type,omitempty"`
	Text       string             `json:"text,omitempty"`
	Image      *notebookReadImage `json:"image,omitempty"`
}

type notebookReadImage struct {
	MediaType string `json:"media_type"`
	Data      string `json:"image_data"`
}

func parseNotebookSourceText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single, nil
	}
	var lines []string
	if err := json.Unmarshal(raw, &lines); err == nil {
		return strings.Join(lines, ""), nil
	}
	return "", i18n.NewError(i18n.KeyToolSourceSinkNotebookSourceFormat)
}

func processNotebookOutputText(raw json.RawMessage) string {
	text, err := parseNotebookSourceText(raw)
	if err != nil {
		return ""
	}
	return text
}

func extractNotebookOutputImage(data map[string]json.RawMessage) *notebookReadImage {
	if len(data) == 0 {
		return nil
	}
	for _, mediaType := range []string{"image/png", "image/jpeg"} {
		raw, ok := data[mediaType]
		if !ok {
			continue
		}
		value := strings.TrimSpace(processNotebookOutputText(raw))
		if value == "" {
			continue
		}
		return &notebookReadImage{MediaType: mediaType, Data: strings.ReplaceAll(value, "\n", "")}
	}
	return nil
}

func processNotebookOutput(output readNotebookOutput) (notebookReadOutput, bool) {
	switch output.OutputType {
	case "stream":
		return notebookReadOutput{
			OutputType: output.OutputType,
			Text:       processNotebookOutputText(output.Text),
		}, true
	case "execute_result", "display_data":
		return notebookReadOutput{
			OutputType: output.OutputType,
			Text:       processNotebookOutputText(output.Data["text/plain"]),
			Image:      extractNotebookOutputImage(output.Data),
		}, true
	case "error":
		text := strings.TrimSpace(fmt.Sprintf("%s: %s\n%s", output.EName, output.EValue, strings.Join(output.Traceback, "\n")))
		return notebookReadOutput{OutputType: output.OutputType, Text: text}, true
	default:
		return notebookReadOutput{}, false
	}
}

func notebookOutputsTooLarge(outputs []notebookReadOutput) bool {
	size := 0
	for _, output := range outputs {
		size += len(output.Text)
		if output.Image != nil {
			size += len(output.Image.Data)
		}
		if size > notebookLargeOutputThreshold {
			return true
		}
	}
	return false
}

func readNotebookCells(data []byte, notebookPath string) ([]notebookReadCell, error) {
	var nb readNotebookJSON
	if err := json.Unmarshal(data, &nb); err != nil {
		return nil, i18n.WrapError(i18n.KeyToolSourceSinkNotebookParse, err)
	}

	language := strings.TrimSpace(nb.Metadata.LanguageInfo.Name)
	if language == "" {
		language = "python"
	}

	cells := make([]notebookReadCell, 0, len(nb.Cells))
	for i, cell := range nb.Cells {
		source, err := parseNotebookSourceText(cell.Source)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyToolSourceSinkNotebookCellSource, err)
		}
		cellID := strings.TrimSpace(cell.ID)
		if cellID == "" {
			cellID = fmt.Sprintf("cell-%d", i)
		}

		processedOutputs := make([]notebookReadOutput, 0, len(cell.Outputs))
		for _, output := range cell.Outputs {
			processed, ok := processNotebookOutput(output)
			if !ok {
				continue
			}
			if processed.Text == "" && processed.Image == nil {
				continue
			}
			processedOutputs = append(processedOutputs, processed)
		}
		if cell.CellType == "code" && len(processedOutputs) > 0 && notebookOutputsTooLarge(processedOutputs) {
			processedOutputs = []notebookReadOutput{{
				Text: fmt.Sprintf(
					"Outputs are too large to include. Use Bash with: cat %q | jq '.cells[%d].outputs'",
					notebookPath,
					i,
				),
			}}
		}

		readCell := notebookReadCell{
			CellType:       cell.CellType,
			CellID:         cellID,
			Source:         source,
			ExecutionCount: cell.ExecutionCount,
			Outputs:        processedOutputs,
		}
		if cell.CellType == "code" {
			readCell.Language = language
		}
		cells = append(cells, readCell)
	}
	return cells, nil
}

func renderNotebookCellText(cell notebookReadCell) string {
	var metadata []string
	if cell.CellType != "" && cell.CellType != "code" {
		metadata = append(metadata, fmt.Sprintf("<cell_type>%s</cell_type>", cell.CellType))
	}
	if cell.CellType == "code" && cell.Language != "" && cell.Language != "python" {
		metadata = append(metadata, fmt.Sprintf("<language>%s</language>", cell.Language))
	}
	return fmt.Sprintf(
		`<cell id="%s">%s%s</cell id="%s">`,
		cell.CellID,
		strings.Join(metadata, ""),
		cell.Source,
		cell.CellID,
	)
}

func notebookCellsToContentBlocks(cells []notebookReadCell) []types.ContentBlock {
	blocks := make([]types.ContentBlock, 0, len(cells))
	var text strings.Builder

	flushText := func() {
		if text.Len() == 0 {
			return
		}
		blocks = append(blocks, types.TextBlock{
			Type: types.ContentTypeText,
			Text: text.String(),
		})
		text.Reset()
	}

	for i, cell := range cells {
		if i > 0 {
			text.WriteString("\n\n")
		}
		text.WriteString(renderNotebookCellText(cell))
		for _, output := range cell.Outputs {
			if output.Text != "" {
				text.WriteString("\n")
				text.WriteString(output.Text)
			}
			if output.Image != nil {
				flushText()
				blocks = append(blocks, types.ImageBlock{
					Type: types.ContentTypeImage,
					Source: &types.ImageSource{
						Type:      "base64",
						MediaType: output.Image.MediaType,
						Data:      output.Image.Data,
					},
				})
			}
		}
	}

	flushText()
	return blocks
}

func estimateNotebookContentTokens(blocks []types.ContentBlock) int {
	total := 0
	for _, block := range blocks {
		switch typed := block.(type) {
		case types.TextBlock:
			total += estimateReadTokens(typed.Text)
		case types.ImageBlock:
			if typed.Source != nil {
				total += int(math.Ceil(float64(len(typed.Source.Data)) * 0.125))
			}
		}
	}
	return total
}
