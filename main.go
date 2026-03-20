package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	openai "github.com/sashabaranov/go-openai"
	"github.com/xuri/excelize/v2"
)

// ///////////////////
// VERSION
// ///////////////////
var version = "dev" // Overridden at build time with -ldflags

func getVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && !strings.HasPrefix(info.Main.Version, "v0.0.0-") {
			return info.Main.Version
		}
	}
	return "dev"
}

// ///////////////////
// HUH THEME
// ///////////////////
func createHuhTheme() *huh.Theme {
	t := huh.ThemeCharm()
	t.Focused.Base = t.Focused.Base.BorderBottom(false).BorderTop(false).BorderLeft(false).BorderRight(false)
	t.Focused.Title = t.Focused.Title.Foreground(colorPrimary).Bold(true)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(colorPrimary)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colorSuccess)
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(colorMuted)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colorPrimary)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(colorMuted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(colorPrimary)
	t.Focused.Description = t.Focused.Description.Foreground(colorMuted)
	return t
}

var formTheme = createHuhTheme()

// ///////////////////
// COLORSCHEME
// ///////////////////
var (
	// Base colors
	colorPrimary  = lipgloss.Color("86")  // Bright cyan
	colorSuccess  = lipgloss.Color("82")  // Bright green
	colorWarning  = lipgloss.Color("214") // Orange/yellow
	colorError    = lipgloss.Color("196") // Bright red
	colorMuted    = lipgloss.Color("245") // Light gray
	colorAccent   = lipgloss.Color("213") // Pink/magenta
	colorBorder   = lipgloss.Color("62")  // Blue-ish border
	colorBgHeader = lipgloss.Color("236") // Dark background

	// Styles
	headerStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	progressStyle = lipgloss.NewStyle().
			Padding(0, 1)

	logStyleTranslating = lipgloss.NewStyle().Foreground(colorWarning)
	logStyleReused      = lipgloss.NewStyle().Foreground(colorPrimary)
	logStyleCopied      = lipgloss.NewStyle().Foreground(colorMuted)
	logStyleError       = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	logStyleSkipped     = lipgloss.NewStyle().Foreground(colorAccent)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	// Box styles for layout
	headerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	statusBoxStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	progressBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(colorBorder).
				Padding(0, 1).
				MarginBottom(1)

	viewportBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder).
				Padding(0, 0)

	footerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(colorBorder).
			Padding(0, 1).
			MarginTop(1)

	successBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSuccess).
			Foreground(colorSuccess).
			Padding(0, 1).
			MarginTop(1)

	errorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorError).
			Foreground(colorError).
			Padding(0, 1)
)

// ///////////////////
// BUBBLETEA MODEL
// ///////////////////
type stats struct {
	translated int
	reused     int
	copied     int
	errors     int
	skipped    int
}

type FileType int

const (
	FileTypeTIA FileType = iota
	FileTypeRockwell
)

func (ft FileType) String() string {
	switch ft {
	case FileTypeTIA:
		return "TIA Portal"
	case FileTypeRockwell:
		return "Rockwell FTView"
	default:
		return "Unknown"
	}
}

func detectFileType(headers []string) FileType {
	// TIA: Has column with asterisk marker (e.g., "en-US*") OR "ref=" pattern
	for _, h := range headers {
		if strings.Contains(h, "*") {
			return FileTypeTIA
		}
		if strings.HasPrefix(h, "ref=") {
			return FileTypeTIA
		}
	}
	// Rockwell: First header is "Server"
	if len(headers) > 0 && headers[0] == "Server" {
		return FileTypeRockwell
	}
	// Default to TIA
	return FileTypeTIA
}

type model struct {
	percent     float64
	logMessages []string
	progressBar progress.Model
	viewport    viewport.Model
	done        bool
	err         error
	ready       bool
	fileName    string
	fileType    FileType
	mode        string
	currentRow  int
	totalRows   int
	stats       stats
	width       int
	height      int
}

type progressMsg float64
type logMsg string
type doneMsg struct{}
type statMsg struct {
	translated int
	reused     int
	copied     int
	errors     int
	skipped    int
}
type fileInfoMsg struct {
	fileName  string
	mode      string
	totalRows int
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.ready {
				m.viewport.ScrollDown(1)
			}
			return m, nil
		case "k", "up":
			if m.ready {
				m.viewport.ScrollUp(1)
			}
			return m, nil
		case "g":
			if m.ready {
				m.viewport.GotoTop()
			}
			return m, nil
		case "G":
			if m.ready {
				m.viewport.GotoBottom()
			}
			return m, nil
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 3
		progressHeight := 3
		footerHeight := 2
		viewportHeight := msg.Height - headerHeight - progressHeight - footerHeight - 4
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		m.viewport = viewport.New(msg.Width-4, viewportHeight)
		m.viewport.SetContent(colorizeLogs(m.logMessages))
		m.ready = true
		return m, nil

	case progress.FrameMsg:
		progressModel, cmd := m.progressBar.Update(msg)
		m.progressBar = progressModel.(progress.Model)
		return m, cmd

	case progressMsg:
		m.percent = float64(msg)
		m.currentRow = int(float64(m.totalRows) * float64(msg))
		return m, m.progressBar.SetPercent(float64(msg))

	case logMsg:
		m.logMessages = append(m.logMessages, string(msg))
		if len(m.logMessages) > 3000 {
			m.logMessages = m.logMessages[1:]
		}
		if m.ready {
			m.viewport.SetContent(colorizeLogs(m.logMessages))
			if !m.done {
				m.viewport.GotoBottom()
			}
		}
		return m, nil

	case statMsg:
		m.stats.translated += msg.translated
		m.stats.reused += msg.reused
		m.stats.copied += msg.copied
		m.stats.errors += msg.errors
		m.stats.skipped += msg.skipped
		return m, nil

	case fileInfoMsg:
		m.fileName = msg.fileName
		m.mode = msg.mode
		m.totalRows = msg.totalRows
		return m, nil

	case doneMsg:
		m.done = true
		m.viewport.GotoBottom()
		return m, nil

	case error:
		m.err = msg
		return m, nil

	default:
		var cmd tea.Cmd
		if m.ready {
			m.viewport, cmd = m.viewport.Update(msg)
		}
		return m, cmd
	}
}

func (m model) View() string {
	if m.err != nil {
		return "\n" + errorBoxStyle.Render(fmt.Sprintf(" Error: %v ", m.err)) + "\n"
	}

	if !m.ready {
		return "\n  Initializing..."
	}

	var b strings.Builder

	// Header section
	b.WriteString(renderHeader(m))
	b.WriteString("\n")

	// Status line
	b.WriteString(renderStatus(m))
	b.WriteString("\n")

	// Progress section
	b.WriteString(renderProgress(m))
	b.WriteString("\n")

	// Viewport (logs) with border
	viewportContent := m.viewport.View()
	b.WriteString(viewportBoxStyle.Render(viewportContent))
	b.WriteString("\n")

	// Footer
	b.WriteString(renderFooter(m))

	return b.String()
}

func renderHeader(m model) string {
	versionStr := getVersion()
	title := fmt.Sprintf("TIA Text Translator %s", versionStr)
	return headerBoxStyle.Render(headerStyle.Render(title))
}

func renderStatus(m model) string {
	modeStr := m.mode
	if modeStr == "" {
		modeStr = "Full"
	}
	fileStr := m.fileName
	if len(fileStr) > 40 {
		fileStr = "..." + fileStr[len(fileStr)-37:]
	}

	// Create a compact status line with separators
	status := fmt.Sprintf("File: %s  |  Mode: %s  |  Rows: %d", fileStr, modeStr, m.totalRows)
	return statusBoxStyle.Render(status)
}

func renderProgress(m model) string {
	percent := int(m.percent * 100)
	progressBar := m.progressBar.View()
	statsLine := fmt.Sprintf("%3d%% (%d/%d)", percent, m.currentRow, m.totalRows)

	// Combine progress bar and stats
	line := fmt.Sprintf("%s  %s", progressBar, statsLine)
	return progressBoxStyle.Render(line)
}

func renderFooter(m model) string {
	if m.done {
		// Summary when complete
		var parts []string
		parts = append(parts, fmt.Sprintf("Translated: %d", m.stats.translated))
		parts = append(parts, fmt.Sprintf("Reused: %d", m.stats.reused))
		parts = append(parts, fmt.Sprintf("Copied: %d", m.stats.copied))
		if m.stats.skipped > 0 {
			parts = append(parts, fmt.Sprintf("Skipped: %d", m.stats.skipped))
		}
		parts = append(parts, fmt.Sprintf("Errors: %d", m.stats.errors))
		summary := "Complete!  " + strings.Join(parts, "  |  ")
		return successBoxStyle.Render(summary)
	}
	// Keyboard shortcuts during translation
	return footerBoxStyle.Render(footerStyle.Render("j/k: scroll  |  G: bottom  |  g: top  |  q: quit"))
}

func colorizeLogs(logs []string) string {
	var colored []string
	for _, msg := range logs {
		colored = append(colored, colorizeLog(msg))
	}
	return strings.Join(colored, "\n")
}

func colorizeLog(msg string) string {
	switch {
	case strings.HasPrefix(msg, "ERROR:"):
		return logStyleError.Render(msg)
	case strings.HasPrefix(msg, "Reused"):
		return logStyleReused.Render(msg)
	case strings.HasPrefix(msg, "Copied") || strings.HasPrefix(msg, "Copying"):
		return logStyleCopied.Render(msg)
	case strings.HasPrefix(msg, "Translating:"):
		return logStyleTranslating.Render(msg)
	case strings.HasPrefix(msg, "Quick mode:"):
		return logStyleSkipped.Render(msg)
	default:
		return msg
	}
}

// displayErrorAndExit shows an error in a TUI interface before exiting
func displayErrorAndExit(err error) {
	// Create a simple TUI to display the error
	errorModel := model{
		err: err,
	}

	p := tea.NewProgram(errorModel)
	if _, runErr := p.Run(); runErr != nil {
		// If TUI fails, fall back to log output
		log.Fatalf("Error displaying error: %v\nOriginal error: %v", runErr, err)
	}

	// Exit with error code after TUI closes
	os.Exit(1)
}

// hasUnderscoreNumberPattern checks if a text follows the pattern base_number
func hasUnderscoreNumberPattern(text string) bool {
	parts := strings.Split(text, "_")
	if len(parts) < 2 {
		return false
	}
	// Check if the last part is a number
	_, err := strconv.Atoi(parts[len(parts)-1])
	return err == nil
}

// extractBaseAndSuffix splits a text into base and suffix parts
// For "Discrete_alarm_66", returns ("Discrete_alarm", "66")
func extractBaseAndSuffix(text string) (string, string) {
	parts := strings.Split(text, "_")
	if len(parts) < 2 {
		return text, ""
	}

	lastIndex := len(parts) - 1
	base := strings.Join(parts[:lastIndex], "_")
	suffix := parts[lastIndex]

	return base, suffix
}

// isVisualSeparator checks if text is mostly visual separators (dashes, underscores, etc.)
func isVisualSeparator(text string) bool {
	if len(text) < 5 {
		return false
	}
	separatorChars := 0
	for _, char := range text {
		if char == '-' || char == '_' || char == '=' || char == '*' || char == '.' {
			separatorChars++
		}
	}
	return float64(separatorChars)/float64(len(text)) >= 0.8
}

func hasSpaceNumberPattern(text string) bool {
	lastSpace := strings.LastIndex(text, " ")
	if lastSpace == -1 || lastSpace == len(text)-1 {
		return false
	}
	_, err := strconv.Atoi(text[lastSpace+1:])
	return err == nil
}

func extractSpaceBaseAndSuffix(text string) (string, string) {
	lastSpace := strings.LastIndex(text, " ")
	if lastSpace == -1 || lastSpace == len(text)-1 {
		return text, ""
	}
	base := text[:lastSpace]
	suffix := text[lastSpace+1:]
	return base, suffix
}

func shouldReuseTranslation(currentText, previousText string) (bool, string, string, string) {
	if hasUnderscoreNumberPattern(currentText) && hasUnderscoreNumberPattern(previousText) {
		currentBase, currentSuffix := extractBaseAndSuffix(currentText)
		previousBase, _ := extractBaseAndSuffix(previousText)
		if currentBase == previousBase {
			return true, currentBase, currentSuffix, "_"
		}
	}
	if hasSpaceNumberPattern(currentText) && hasSpaceNumberPattern(previousText) {
		currentBase, currentSuffix := extractSpaceBaseAndSuffix(currentText)
		previousBase, _ := extractSpaceBaseAndSuffix(previousText)
		if currentBase == previousBase {
			return true, currentBase, currentSuffix, " "
		}
	}
	if strings.Contains(currentText, "#") && strings.Contains(previousText, "#") {
		currentParts := strings.SplitN(currentText, "#", 2)
		previousParts := strings.SplitN(previousText, "#", 2)
		if len(currentParts) == 2 && len(previousParts) == 2 && currentParts[0] == previousParts[0] {
			return true, strings.TrimSpace(currentParts[0]), strings.TrimSpace(currentParts[1]), "#"
		}
	}
	return false, "", "", ""
}

func extractTranslatedBase(translation, delim string) string {
	switch delim {
	case "_":
		base, _ := extractBaseAndSuffix(translation)
		return base
	case " ":
		base, _ := extractSpaceBaseAndSuffix(translation)
		return base
	case "#":
		parts := strings.SplitN(translation, "#", 2)
		if len(parts) == 2 {
			return parts[0]
		}
		return translation
	default:
		return translation
	}
}

// hasEmbeddedRefs checks if text contains /*...*/ style embedded references
func hasEmbeddedRefs(text string) bool {
	return strings.Contains(text, "/*") && strings.Contains(text, "*/")
}

// embeddedRefRegex matches /*...*/ patterns
var embeddedRefRegex = regexp.MustCompile(`/\*[^*]*\*/`)

// extractEmbeddedRefs extracts all embedded ref segments from text
func extractEmbeddedRefs(text string) []string {
	return embeddedRefRegex.FindAllString(text, -1)
}

// splitTextByRefs splits text into alternating segments of text and refs
// Even indices are text (translatable), odd indices are refs (preserve as-is)
func splitTextByRefs(text string) []string {
	if !hasEmbeddedRefs(text) {
		return []string{text}
	}

	var segments []string
	lastEnd := 0

	matches := embeddedRefRegex.FindAllStringIndex(text, -1)

	for _, match := range matches {
		start, end := match[0], match[1]
		// Add text before this ref (even if empty)
		segments = append(segments, text[lastEnd:start])
		// Add the ref itself
		segments = append(segments, text[start:end])
		lastEnd = end
	}

	// Add any remaining text after the last ref (even if empty)
	segments = append(segments, text[lastEnd:])

	return segments
}

// reassembleWithRefs joins segments back together, preserving refs
func reassembleWithRefs(segments []string) string {
	return strings.Join(segments, "")
}

func main() {
	// ///////////////////
	// 1. GET USER INPUT
	// ///////////////////
	csvOutput := flag.Bool("csv", false, "Output to a CSV file instead of XLSX for debugging.")
	flag.Parse()

	apiKey, err := getAPIKey()
	if err != nil {
		displayErrorAndExit(err)
	}

	if err := validateAPIKey(apiKey); err != nil {
		displayErrorAndExit(fmt.Errorf("API key validation failed: %v. Please check your key and try again.", err))
	}

	// Find both .xls and .xlsx files
	xlsxFiles, err := filepath.Glob("*.xlsx")
	if err != nil {
		displayErrorAndExit(fmt.Errorf("Error finding .xlsx files: %v", err))
	}
	xlsFiles, err := filepath.Glob("*.xls")
	if err != nil {
		displayErrorAndExit(fmt.Errorf("Error finding .xls files: %v", err))
	}
	files := append(xlsxFiles, xlsFiles...)

	var filteredFiles []string
	for _, file := range files {
		if !strings.HasPrefix(file, "translated-") {
			filteredFiles = append(filteredFiles, file)
		}
	}

	if len(filteredFiles) == 0 {
		displayErrorAndExit(fmt.Errorf("No .xls or .xlsx files found to translate."))
	}

	// Print welcome header
	fmt.Println()
	fmt.Println(headerBoxStyle.Render(headerStyle.Render(fmt.Sprintf("TIA Text Translator %s", getVersion()))))
	fmt.Println()
	fmt.Println(statusStyle.Render("Select options to begin translation..."))
	fmt.Println()

	var fileName string
	var sourceLangIndex, targetLangIndex int
	var translationMode string

	fileOptions := make([]huh.Option[string], len(filteredFiles))
	for i, f := range filteredFiles {
		fileOptions[i] = huh.NewOption(f, f)
	}

	form := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().Title("Select a file to translate").Options(fileOptions...).Value(&fileName)),
	).WithTheme(formTheme)

	if err := form.Run(); err != nil {
		displayErrorAndExit(err)
	}

	f, err := excelize.OpenFile(fileName)
	if err != nil {
		displayErrorAndExit(fmt.Errorf("Error opening file: %v", err))
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		displayErrorAndExit(fmt.Errorf("Error getting rows: %v", err))
	}
	headers := rows[0]

	// Detect file type from headers
	fileType := detectFileType(headers)
	fmt.Println()
	fmt.Println(statusBoxStyle.Render(fmt.Sprintf("Detected: %s", fileType.String())))
	fmt.Println()

	// Determine metadata columns based on file type
	var metadataCols int
	var skipRefColumns bool
	switch fileType {
	case FileTypeTIA:
		metadataCols = 4
		skipRefColumns = true
	case FileTypeRockwell:
		metadataCols = 5 // Server, Component Type, Component Name, Description, REF
		skipRefColumns = false
	}

	// Build column options, skipping metadata and optionally ref columns
	var colOptions []huh.Option[int]
	for i, h := range headers {
		if i < metadataCols {
			continue // Skip metadata
		}
		if skipRefColumns && strings.HasPrefix(strings.ToLower(h), "ref=") {
			continue // Skip ref columns in TIA
		}
		colOptions = append(colOptions, huh.NewOption(fmt.Sprintf("%s (Col %d)", h, i+1), i))
	}

	modeOptions := []huh.Option[string]{
		huh.NewOption("Full (translate all)", "full").Selected(true),
		huh.NewOption("Quick (only empty/placeholder target texts)", "quick"),
	}

	setupForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().Title("Select Source Language Column").Options(colOptions...).Value(&sourceLangIndex),
			huh.NewSelect[int]().Title("Select Target Language Column").Options(colOptions...).Value(&targetLangIndex),
			huh.NewSelect[string]().Title("Select Translation Mode").Options(modeOptions...).Value(&translationMode),
		),
	).WithTheme(formTheme)

	if err := setupForm.Run(); err != nil {
		displayErrorAndExit(err)
	}

	// Show summary screen
	summaryLines := []string{
		fmt.Sprintf("File:       %s", fileName),
		fmt.Sprintf("Type:       %s", fileType.String()),
		fmt.Sprintf("Source:     %s (Column %d)", headers[sourceLangIndex], sourceLangIndex+1),
		fmt.Sprintf("Target:     %s (Column %d)", headers[targetLangIndex], targetLangIndex+1),
		fmt.Sprintf("Mode:       %s", map[string]string{"full": "Full", "quick": "Quick"}[translationMode]),
		fmt.Sprintf("Total rows: %d", len(rows)-1), // -1 for header
	}
	summaryText := strings.Join(summaryLines, "\n")

	confirmVar := true
	summaryForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Translation Summary").
				Description(summaryText).
				Affirmative("Start Translation").
				Negative("Cancel").
				Value(&confirmVar),
		),
	).WithTheme(formTheme)

	if err := summaryForm.Run(); err != nil {
		displayErrorAndExit(err)
	}

	if !confirmVar {
		fmt.Println("\nTranslation cancelled.")
		os.Exit(0)
	}

	// ///////////////////
	// 2. RUN TRANSLATION WITH TUI
	// ///////////////////
	m := model{
		progressBar: progress.New(progress.WithDefaultGradient()),
		fileName:    fileName,
		fileType:    fileType,
		mode:        translationMode,
		totalRows:   len(rows),
	}
	p := tea.NewProgram(m, tea.WithAltScreen())

	go iterateAndTranslate(p, apiKey, f, sheetName, rows, sourceLangIndex, targetLangIndex, headers[sourceLangIndex], headers[targetLangIndex], translationMode, fileType)

	if _, err := p.Run(); err != nil {
		displayErrorAndExit(fmt.Errorf("Error running program: %v", err))
	}

	// ///////////////////
	// 3. SAVE FILE
	// ///////////////////
	baseName := "translated-" + strings.TrimSuffix(fileName, filepath.Ext(fileName))
	var newFileName string

	if *csvOutput {
		newFileName = baseName + ".csv"
		if err := saveAsCSV(f, sheetName, newFileName); err != nil {
			displayErrorAndExit(fmt.Errorf("Error saving new CSV file: %v", err))
		}
	} else {
		newFileName = baseName + ".xlsx"
		if err := f.SaveAs(newFileName); err != nil {
			displayErrorAndExit(fmt.Errorf("Error saving new XLSX file: %v", err))
		}
	}

	fmt.Println(successBoxStyle.Render(fmt.Sprintf("Translation saved to %s", newFileName)))
}

func translateText(client *openai.Client, text, sourceLang, targetLang string) (string, error) {
	prompt := fmt.Sprintf("You are a professional translator. Translate the following text from '%s' to '%s'. Do not add any extra conversational text or quotation marks, just provide the translation. If the text is a placeholder or code, return it as is. The text to translate is: %s", sourceLang, targetLang, text)
	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: openai.GPT4oMini,
		Messages: []openai.ChatCompletionMessage{{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		}},
	})
	if err != nil {
		return "", err
	}
	translation := resp.Choices[0].Message.Content
	return strings.Trim(translation, "\""), nil
}

var meaninglessAlarmRegex = regexp.MustCompile(`(?i)^alarm\s+\d+:\s*$`) // For alarms like "Alarm 16: "

func isPlaceholder(text string) bool {
	switch {
	case strings.HasPrefix(text, "##") && strings.HasSuffix(text, "##"):
		return true
	case strings.HasPrefix(text, "#") && strings.HasSuffix(text, "#") && len(text) > 1:
		return true
	case strings.HasPrefix(text, "@") && strings.HasSuffix(text, "@"):
		return true
	case meaninglessAlarmRegex.MatchString(text):
		return true
	default:
		return false
	}
}

func saveAsCSV(f *excelize.File, sheetName, newFileName string) error {
	file, err := os.Create(newFileName)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return fmt.Errorf("failed to get rows from sheet: %w", err)
	}

	return writer.WriteAll(rows)
}

// getAPIKey retrieves the OpenAI API key from one of the following sources
// in order:
// 1. OPENAI_API_KEY environment variable
// 2. api-key.txt file in the executable's directory
// 3. User prompt
func getAPIKey() (string, error) {
	// 1. Check environment variable
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key, nil
	}

	// 2. Check for api-key.txt file
	// Get the directory of the executable
	ex, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not get executable path: %w", err)
	}
	exPath := filepath.Dir(ex)
	keyPath := filepath.Join(exPath, "api-key.txt")

	// Check if the file exists and read it
	if _, err := os.Stat(keyPath); err == nil {
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			// If we can't read it, we'll just proceed to prompt the user.
			// We can log this for debugging if needed, but for the user, it's not a fatal error.
		} else {
			key := strings.TrimSpace(string(keyBytes))
			if key != "" {
				return key, nil
			}
		}
	}

	// 3. Prompt user for key
	var apiKey string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("OpenAI API Key Required").
				Description("Enter your OpenAI API key (not stored).").
				Value(&apiKey).
				Password(true),
		),
	).WithTheme(formTheme)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("could not get API key from user: %w", err)
	}

	if apiKey == "" {
		return "", fmt.Errorf("API key cannot be empty")
	}

	return apiKey, nil
}

// validateAPIKey makes a lightweight call to OpenAI to ensure the key is valid.
func validateAPIKey(apiKey string) error {
	client := openai.NewClient(apiKey)
	// A simple, low-cost request to check for authentication.
	_, err := client.ListModels(context.Background())
	if err != nil {
		// Check for a specific 401 Unauthorized error.
		if apiErr, ok := err.(*openai.APIError); ok && apiErr.HTTPStatusCode == http.StatusUnauthorized {
			return fmt.Errorf("the provided API key is invalid or has expired")
		}
		// Return a more generic error for other issues (e.g., network problems).
		return fmt.Errorf("could not connect to OpenAI: %w", err)
	}
	return nil
}

func iterateAndTranslate(p *tea.Program, apiKey string, f *excelize.File, sheetName string, rows [][]string, sourceIndex, targetIndex int, sourceLang, targetLang string, translationMode string, fileType FileType) {
	var stats struct {
		translated int
		reused     int
		copied     int
		errors     int
		skipped    int
	}
	defer func() {
		p.Send(statMsg{
			translated: stats.translated,
			reused:     stats.reused,
			copied:     stats.copied,
			errors:     stats.errors,
			skipped:    stats.skipped,
		})
		if stats.skipped > 0 {
			p.Send(logMsg(fmt.Sprintf("Skipped %d rows in quick mode.", stats.skipped)))
		}
		p.Send(doneMsg{})
	}()

	client := openai.NewClient(apiKey)
	var previousText, previousTranslation string
	totalRows := len(rows)

	for i, row := range rows {
		p.Send(progressMsg(float64(i+1) / float64(totalRows))) // Update progress

		if i == 0 { // Skip header row
			continue
		}

		if len(row) <= sourceIndex {
			continue
		}

		sourceText := strings.TrimSpace(row[sourceIndex])
		var targetText string
		if len(row) > targetIndex {
			targetText = strings.TrimSpace(row[targetIndex])
		}

		// Rockwell-specific: Handle **REF:N** patterns
		if fileType == FileTypeRockwell {
			// Check if source is a REF field
			isSourceRef := strings.HasPrefix(sourceText, "**REF:") && strings.HasSuffix(sourceText, "**")
			// Check if target is a REF field
			isTargetRef := strings.HasPrefix(targetText, "**REF:") && strings.HasSuffix(targetText, "**")

			if isSourceRef {
				if isTargetRef {
					// Both source and target are REF fields - skip
					p.Send(logMsg(fmt.Sprintf("Rockwell: Skipping REF field (both source and target have REF)")))
					continue
				} else if targetText == "" {
					// Source has REF, target is empty - copy source to target
					cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
					f.SetCellValue(sheetName, cell, sourceText)
					p.Send(logMsg(fmt.Sprintf("Rockwell: Copied REF to target: %s", sourceText)))
					stats.copied++
					time.Sleep(10 * time.Millisecond)
					continue
				}
				// Source has REF, target has non-REF content - proceed to check if we should translate
			}

			// If target already has a REF, skip this row
			if isTargetRef {
				p.Send(logMsg(fmt.Sprintf("Rockwell: Skipping row (target has REF): %s", targetText)))
				continue
			}

			// Rockwell-specific: Handle embedded refs /*...*/
			if hasEmbeddedRefs(sourceText) {
				// In quick mode, if target has same refs pattern, skip
				if translationMode == "quick" && hasEmbeddedRefs(targetText) {
					p.Send(logMsg(fmt.Sprintf("Rockwell: Skipping row (target already has embedded refs)")))
					stats.skipped++
					continue
				}

				// Split source into segments
				segments := splitTextByRefs(sourceText)
				var translatedSegments []string

				for idx, segment := range segments {
					if idx%2 == 0 {
						// Even indices: text segment (translatable)
						trimmed := strings.TrimSpace(segment)
						if trimmed == "" {
							translatedSegments = append(translatedSegments, segment)
							continue
						}

						// Translate this text segment
						p.Send(logMsg(fmt.Sprintf("Rockwell: Translating segment: %s", trimmed)))
						translated, err := translateText(client, trimmed, sourceLang, targetLang)
						if err != nil {
							p.Send(logMsg(fmt.Sprintf("ERROR: %v", err)))
							translatedSegments = append(translatedSegments, segment)
							stats.errors++
						} else {
							// Preserve spacing from original
							if strings.HasPrefix(segment, " ") && !strings.HasPrefix(translated, " ") {
								translated = " " + translated
							}
							if strings.HasSuffix(segment, " ") && !strings.HasSuffix(translated, " ") {
								translated = translated + " "
							}
							translatedSegments = append(translatedSegments, translated)
							stats.translated++
						}
					} else {
						// Odd indices: ref segment (preserve as-is)
						translatedSegments = append(translatedSegments, segment)
					}
				}

				// Reassemble and save
				translatedText := reassembleWithRefs(translatedSegments)
				cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
				f.SetCellValue(sheetName, cell, translatedText)
				p.Send(logMsg(fmt.Sprintf("Rockwell: Saved with embedded refs")))
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		// Skip rows with empty source AND empty target
		if sourceText == "" && targetText == "" {
			continue
		}

		// Skip translating the default "Text" value from TIA Portal.
		if strings.EqualFold(sourceText, "Text") {
			continue
		}

		if isPlaceholder(sourceText) {
			p.Send(logMsg(fmt.Sprintf("Copied placeholder: %s", sourceText)))
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, sourceText)
			stats.copied++
			time.Sleep(10 * time.Millisecond) // Slow down for UI
			continue
		}

		// Copy short texts and numerals in both modes
		if len(sourceText) < 3 || (len(sourceText) > 0 && sourceText[0] == '!') {
			p.Send(logMsg(fmt.Sprintf("Copying short text: %s", sourceText)))
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, sourceText)
			stats.copied++
			time.Sleep(10 * time.Millisecond) // Slow down for UI
			continue
		}
		if _, err := strconv.Atoi(sourceText); err == nil {
			p.Send(logMsg(fmt.Sprintf("Copying numeral: %s", sourceText)))
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, sourceText)
			stats.copied++
			time.Sleep(10 * time.Millisecond) // Slow down for UI
			continue
		}

		// Skip visual separators (mostly dashes, underscores, etc.)
		if isVisualSeparator(sourceText) {
			p.Send(logMsg(fmt.Sprintf("Skipping visual separator: %s", sourceText)))
			continue
		}

		// Quick mode: Only translate if target cell is empty or just "Text"
		if translationMode == "quick" {
			if len(row) > targetIndex {
				targetTextForCheck := strings.ToLower(strings.Trim(targetText, `"`))

				// Skip if target has meaningful content (not empty and not "text")
				shouldSkip := targetTextForCheck != "" && targetTextForCheck != "text"

				if shouldSkip {
					p.Send(logMsg(fmt.Sprintf("Quick mode: skipping row %d", i+1)))
					stats.skipped++
					continue
				}
			}
		}

		var translatedText string
		var err error
		var isReused bool

		// If current text is exactly the same as previous text, reuse translation
		if sourceText == previousText && previousTranslation != "" {
			translatedText = previousTranslation
			p.Send(logMsg(fmt.Sprintf("Reused identical translation for: %s", sourceText)))
			isReused = true
			goto saveAndContinue
		}

		// Check if we can reuse translation based on pattern matching
		if shouldReuse, _, currentSuffix, delim := shouldReuseTranslation(sourceText, previousText); shouldReuse {
			translatedPreviousBase := extractTranslatedBase(previousTranslation, delim)
			if _, err := strconv.Atoi(currentSuffix); err == nil {
				// Suffix is a number, reuse the translated base
				translatedText = translatedPreviousBase + delim + currentSuffix
				p.Send(logMsg(fmt.Sprintf("Reused base for: %s", sourceText)))
				isReused = true
			} else {
				// Suffix is not a number, translate it
				p.Send(logMsg(fmt.Sprintf("Translating suffix: %s", currentSuffix)))
				suffixTranslation, err := translateText(client, currentSuffix, sourceLang, targetLang)
				if err != nil {
					p.Send(logMsg(fmt.Sprintf("ERROR: %v", err)))
					translatedText = sourceText
					stats.errors++
				} else {
					translatedText = translatedPreviousBase + delim + suffixTranslation
					stats.translated++
				}
			}
			goto saveAndContinue
		}

		p.Send(logMsg(fmt.Sprintf("Translating: %s", sourceText)))
		translatedText, err = translateText(client, sourceText, sourceLang, targetLang)
		if err != nil {
			p.Send(logMsg(fmt.Sprintf("ERROR: %v", err)))
			stats.errors++
			continue
		}
		stats.translated++

	saveAndContinue:
		cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
		f.SetCellValue(sheetName, cell, translatedText)

		if isReused {
			stats.reused++
		}

		previousText = sourceText
		previousTranslation = translatedText
		time.Sleep(50 * time.Millisecond) // Rate limit and slow down for UI
	}
}
