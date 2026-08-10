package localbench

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/agent-dance/luban/i18n"
)

//go:embed report.html.tmpl codex-report.html.tmpl
var reportAssets embed.FS

type reportLink struct {
	Label string
	Href  string
}

type reportRun struct {
	Task          TaskSelection
	Agent         string
	Run           *RunSummary
	Evaluation    *Evaluation
	Links         []reportLink
	CacheHitRatio float64
}

type reportTask struct {
	Task TaskSelection
	Runs []reportRun
}

type reportData struct {
	Result          BenchmarkResult
	HTMLLanguage    string
	EvidencePath    string
	Complete        bool
	Codex           Aggregate
	Luban           Aggregate
	Tasks           []reportTask
	SharedCodex     Aggregate
	SharedLuban     Aggregate
	HasSharedPass   bool
	SharedTaskCount int
	HasAdjudication bool
}

type codexReportData struct {
	Snapshot        CodexBaselineSnapshot
	HTMLLanguage    string
	EvidencePath    string
	Aggregate       Aggregate
	Tasks           []reportRun
	HasAdjudication bool
}

func GenerateReport(inputPath, outputPath string, language i18n.Language) error {
	input, err := loadResult(inputPath)
	if err != nil {
		return err
	}
	data, err := compileReport(input, filepath.Dir(outputPath), inputPath, language)
	if err != nil {
		return err
	}
	parent := filepath.Dir(outputPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".benchmark-report-*.html")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := renderReport(temporary, data, language); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return err
	}
	committed = true
	return nil
}

func GenerateCodexReport(snapshot CodexBaselineSnapshot, resultsRoot, outputPath string, language i18n.Language) error {
	outputDir := filepath.Dir(outputPath)
	sourceRoot := filepath.Join(resultsRoot, filepath.FromSlash(snapshot.SourceRunPath))
	snapshotPath := filepath.Join(sourceRoot, currentCodexBaselineJSON)
	if filepath.Clean(outputDir) == filepath.Clean(resultsRoot) {
		snapshotPath = filepath.Join(resultsRoot, currentCodexBaselineJSON)
	}
	data := codexReportData{
		Snapshot: snapshot, HTMLLanguage: languageCode(language),
		EvidencePath: relativeLink(outputDir, snapshotPath), Aggregate: snapshot.Aggregate,
	}
	runs := make(map[string]RunSummary, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runs[run.InstanceID] = run
	}
	evaluations := make(map[string]Evaluation, len(snapshot.Evaluations))
	for _, evaluation := range snapshot.Evaluations {
		evaluations[evaluation.InstanceID] = evaluation
	}
	for _, task := range snapshot.Tasks {
		entry := reportRun{Task: task, Agent: "codex"}
		if run, ok := runs[task.InstanceID]; ok {
			copyValue := run
			entry.Run = &copyValue
			if run.Usage.InputTokens > 0 {
				entry.CacheHitRatio = float64(run.Usage.CachedInputTokens) / float64(run.Usage.InputTokens)
			}
			for _, name := range []string{"summary.json", "events.jsonl", "provider-requests.jsonl", "model.patch"} {
				target := filepath.Join(sourceRoot, filepath.FromSlash(run.EvidenceRoot), name)
				entry.Links = append(entry.Links, reportLink{Label: name, Href: relativeLink(outputDir, target)})
			}
		}
		if evaluation, ok := evaluations[task.InstanceID]; ok {
			copyValue := evaluation
			entry.Evaluation = &copyValue
			evidenceFile := evaluationEvidenceFile(evaluation)
			target := filepath.Join(sourceRoot, filepath.FromSlash(evaluation.EvidenceRoot), evidenceFile)
			entry.Links = append(entry.Links, reportLink{Label: "evaluation/" + evidenceFile, Href: relativeLink(outputDir, target)})
			if evaluation.Adjudicated {
				data.HasAdjudication = true
				for _, name := range []string{"report.json", "adjudication.json"} {
					target := filepath.Join(sourceRoot, filepath.FromSlash(evaluation.EvidenceRoot), name)
					entry.Links = append(entry.Links, reportLink{Label: "evaluation/" + name, Href: relativeLink(outputDir, target)})
				}
			}
		}
		data.Tasks = append(data.Tasks, entry)
	}
	return writeHTMLAtomic(outputPath, func(writer io.Writer) error {
		return renderCodexReport(writer, data, language)
	})
}

func loadResult(path string) (BenchmarkResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return BenchmarkResult{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var result BenchmarkResult
	if err := decoder.Decode(&result); err != nil {
		return BenchmarkResult{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return BenchmarkResult{}, errors.New("benchmark result contains trailing JSON")
	}
	if result.SchemaVersion != ResultSchemaVersion || len(result.Tasks) == 0 || result.Model != ModelID || result.ReasoningEffort != ReasoningEffort {
		return BenchmarkResult{}, errors.New("benchmark result identity is invalid")
	}
	return result, nil
}

func compileReport(result BenchmarkResult, outputDir, inputPath string, language i18n.Language) (reportData, error) {
	data := reportData{
		Result: result, HTMLLanguage: languageCode(language), Complete: result.Status == "complete",
		EvidencePath: relativeLink(outputDir, inputPath),
	}
	for _, aggregate := range result.Aggregates {
		switch aggregate.Agent {
		case "codex":
			data.Codex = aggregate
		case "luban":
			data.Luban = aggregate
		}
	}
	runs := make(map[string]RunSummary, len(result.Runs))
	for _, run := range result.Runs {
		runs[run.InstanceID+"/"+run.Agent] = run
	}
	evaluations := make(map[string]Evaluation, len(result.Evaluations))
	for _, evaluation := range result.Evaluations {
		evaluations[evaluation.InstanceID+"/"+evaluation.Agent] = evaluation
	}
	for _, task := range result.Tasks {
		row := reportTask{Task: task}
		for _, agent := range []string{"codex", "luban"} {
			key := task.InstanceID + "/" + agent
			entry := reportRun{Task: task, Agent: agent}
			if value, ok := runs[key]; ok {
				copyValue := value
				entry.Run = &copyValue
				if value.Usage.InputTokens > 0 {
					entry.CacheHitRatio = float64(value.Usage.CachedInputTokens) / float64(value.Usage.InputTokens)
				}
				for _, name := range []string{"summary.json", "events.jsonl", "provider-requests.jsonl", "model.patch"} {
					entry.Links = append(entry.Links, reportLink{Label: name, Href: filepath.ToSlash(filepath.Join(value.EvidenceRoot, name))})
				}
			}
			if value, ok := evaluations[key]; ok {
				copyValue := value
				entry.Evaluation = &copyValue
				evidenceFile := evaluationEvidenceFile(value)
				entry.Links = append(entry.Links, reportLink{Label: "evaluation/" + evidenceFile, Href: filepath.ToSlash(filepath.Join(value.EvidenceRoot, evidenceFile))})
				if value.Adjudicated {
					data.HasAdjudication = true
					for _, name := range []string{"report.json", "adjudication.json"} {
						entry.Links = append(entry.Links, reportLink{Label: "evaluation/" + name, Href: filepath.ToSlash(filepath.Join(value.EvidenceRoot, name))})
					}
				}
			}
			row.Runs = append(row.Runs, entry)
		}
		data.Tasks = append(data.Tasks, row)
	}
	if data.Complete && len(result.SharedPass) > 0 {
		sharedSet := make(map[string]struct{}, len(result.SharedPass))
		for _, task := range result.SharedPass {
			sharedSet[task.InstanceID] = struct{}{}
		}
		data.SharedCodex = aggregateSubset("codex", result.Tasks, result.Runs, result.Evaluations, sharedSet)
		data.SharedLuban = aggregateSubset("luban", result.Tasks, result.Runs, result.Evaluations, sharedSet)
		data.HasSharedPass = true
		data.SharedTaskCount = len(sharedSet)
	}
	return data, nil
}

func evaluationEvidenceFile(value Evaluation) string {
	if value.EvidenceFile != "" {
		return value.EvidenceFile
	}
	return "report.json"
}

func renderReport(writer io.Writer, data reportData, language i18n.Language) error {
	templateValue, err := template.New("benchmark-report").Funcs(reportTemplateFunctions(language)).ParseFS(reportAssets, "report.html.tmpl")
	if err != nil {
		return err
	}
	return templateValue.ExecuteTemplate(writer, "report.html.tmpl", data)
}

func renderCodexReport(writer io.Writer, data codexReportData, language i18n.Language) error {
	templateValue, err := template.New("codex-report").Funcs(reportTemplateFunctions(language)).ParseFS(reportAssets, "codex-report.html.tmpl")
	if err != nil {
		return err
	}
	return templateValue.ExecuteTemplate(writer, "codex-report.html.tmpl", data)
}

func reportTemplateFunctions(language i18n.Language) template.FuncMap {
	return template.FuncMap{
		"tr":        func(key string) string { return i18n.Text(language, i18n.Key(key)) },
		"tf":        func(key string, arguments ...any) string { return i18n.Format(language, i18n.Key(key), arguments...) },
		"seconds":   secondsValue,
		"cost":      func(value float64) string { return fmt.Sprintf("$%.4f", value) },
		"percent":   func(value float64) string { return fmt.Sprintf("%.1f%%", value*100) },
		"timeValue": func(value time.Time) string { return value.UTC().Format(time.RFC3339) },
		"status": func(value bool) string {
			if value {
				return i18n.Text(language, i18n.KeyAgenticReportStatusPass)
			}
			return i18n.Text(language, i18n.KeyAgenticReportStatusFail)
		},
		"statusClass": func(value bool) string {
			if value {
				return "good"
			}
			return "bad"
		},
		"score": func(passed, total int) string {
			return fmt.Sprintf("%d/%d (%.1f%%)", passed, total, 100*float64(passed)/float64(max(total, 1)))
		},
		"toolSummary": toolSummary,
		"list":        func(values ...Aggregate) []Aggregate { return values },
	}
}

func writeHTMLAtomic(outputPath string, render func(io.Writer) error) error {
	parent := filepath.Dir(outputPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".benchmark-report-*.html")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := render(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return err
	}
	committed = true
	return nil
}

func toolSummary(values map[string]int) string {
	type pair struct {
		name  string
		count int
	}
	items := make([]pair, 0, len(values))
	for name, count := range values {
		items = append(items, pair{name, count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].name < items[j].name
	})
	result := ""
	for index, item := range items {
		if index > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%s=%d", item.name, item.count)
	}
	return result
}

func secondsValue(value float64) string {
	minutes := int(value) / 60
	seconds := value - float64(minutes*60)
	if minutes > 0 {
		return fmt.Sprintf("%dm %.1fs", minutes, seconds)
	}
	return fmt.Sprintf("%.1fs", seconds)
}

func languageCode(language i18n.Language) string {
	if language == i18n.LangZH {
		return "zh-CN"
	}
	return language.Code()
}

func relativeLink(base, target string) string {
	relative, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.Base(target)
	}
	return filepath.ToSlash(relative)
}
