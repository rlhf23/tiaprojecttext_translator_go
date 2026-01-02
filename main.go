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
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	openai "github.com/sashabaranov/go-openai"
	"github.com/xuri/excelize/v2"
)

// ///////////////////
// TUI STYLES
// ///////////////////
var (
	docStyle    = lipgloss.NewStyle().Margin(1, 2)
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errMsgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// ///////////////////
// BUBBLETEA MODEL
// ///////////////////
type model struct {
	percent     float64
	logMessages []string
	progressBar progress.Model
	done        bool
	err         error
}

type progressMsg float64
type logMsg string
type doneMsg struct{}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m, tea.Quit

	case progress.FrameMsg:
		progressModel, cmd := m.progressBar.Update(msg)
		m.progressBar = progressModel.(progress.Model)
		return m, cmd

	case progressMsg:
		m.percent = float64(msg)
		return m, m.progressBar.SetPercent(float64(msg))

	case logMsg:
		m.logMessages = append(m.logMessages, string(msg))
		if len(m.logMessages) > 50 {
			m.logMessages = m.logMessages[1:]
		}
		return m, nil

	case doneMsg:
		m.done = true
		// Wait 500ms before quitting to ensure final messages are displayed.
		return m, func() tea.Msg {
			time.Sleep(500 * time.Millisecond)
			return tea.Quit()
		}

	case error:
		m.err = msg
		return m, tea.Quit

	default:
		return m, nil
	}
}

func (m model) View() string {
	if m.err != nil {
		return docStyle.Render(errMsgStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	progressView := m.progressBar.View() + "\n\n"

	logs := strings.Join(m.logMessages, "\n")

	var help string
	if !m.done {
		help = helpStyle.Render("Translating... Press any key to quit.")
	} else {
		help = helpStyle.Render("Translation complete!")
	}

	return docStyle.Render(progressView + logs + "\n\n" + help)
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

	files, err := filepath.Glob("*.xlsx")
	if err != nil {
		displayErrorAndExit(fmt.Errorf("Error finding .xlsx files: %v", err))
	}

	var filteredFiles []string
	for _, file := range files {
		if !strings.HasPrefix(file, "translated-") {
			filteredFiles = append(filteredFiles, file)
		}
	}

	if len(filteredFiles) == 0 {
		displayErrorAndExit(fmt.Errorf("No .xlsx files found to translate."))
	}

	var fileName string
	var sourceLangIndex, targetLangIndex int
	var translationMode string

	fileOptions := make([]huh.Option[string], len(filteredFiles))
	for i, f := range filteredFiles {
		fileOptions[i] = huh.NewOption(f, f)
	}

	form := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().Title("Select a file to translate").Options(fileOptions...).Value(&fileName)),
	)

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
	var colOptions []huh.Option[int]
	for i, h := range headers {
		// Skip the first 4 columns (metadata) and any reference columns.
		if i >= 4 && !strings.HasPrefix(strings.ToLower(h), "ref=") {
			colOptions = append(colOptions, huh.NewOption(fmt.Sprintf("%s (Col %d)", h, i+1), i))
		}
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
	)

	if err := setupForm.Run(); err != nil {
		displayErrorAndExit(err)
	}

	// ///////////////////
	// 2. RUN TRANSLATION WITH TUI
	// ///////////////////
	m := model{
		progressBar: progress.New(progress.WithDefaultGradient()),
	}
	p := tea.NewProgram(m)

	go iterateAndTranslate(p, apiKey, f, sheetName, rows, sourceLangIndex, targetLangIndex, headers[sourceLangIndex], headers[targetLangIndex], translationMode)

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

	fmt.Println(helpStyle.Render(fmt.Sprintf("\nTranslation saved to %s", newFileName)))
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
				Title("OpenAI API Key Not Found").
				Description("Please enter your OpenAI API key.\nIt will not be stored, only used for this session.").
				Value(&apiKey).
				Password(true),
		),
	)

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

func iterateAndTranslate(p *tea.Program, apiKey string, f *excelize.File, sheetName string, rows [][]string, sourceIndex, targetIndex int, sourceLang, targetLang string, translationMode string) {
	var skippedCount int
	defer func() {
		if skippedCount > 0 {
			p.Send(logMsg(fmt.Sprintf("Skipped %d rows in quick mode.", skippedCount)))
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

		text := strings.TrimSpace(row[sourceIndex])

		// Skip translating the default "Text" value from TIA Portal.
		if strings.EqualFold(text, "Text") {
			continue
		}

		if isPlaceholder(text) {
			p.Send(logMsg(fmt.Sprintf("Copied placeholder: %s", text)))
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, text)
			time.Sleep(10 * time.Millisecond) // Slow down for UI
			continue
		}

		// Copy short texts and numerals in both modes
		if len(text) < 3 || (len(text) > 0 && text[0] == '!') {
			p.Send(logMsg(fmt.Sprintf("Copying short text: %s", text)))
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, text)
			time.Sleep(10 * time.Millisecond) // Slow down for UI
			continue
		}
		if _, err := strconv.Atoi(text); err == nil {
			p.Send(logMsg(fmt.Sprintf("Copying numeral: %s", text)))
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, text)
			time.Sleep(10 * time.Millisecond) // Slow down for UI
			continue
		}

		// Quick mode: Only translate if target cell is empty or just "Text"
		if translationMode == "quick" {
			if len(row) > targetIndex {
				targetText := strings.TrimSpace(row[targetIndex])
				// Remove quotes and convert to lowercase for comparison
				targetTextForCheck := strings.ToLower(strings.Trim(targetText, `"`))

				// Debug logging for all quick mode checks
				// p.Send(logMsg(fmt.Sprintf("Quick mode check: target=%q, processed=%q", targetText, targetTextForCheck)))

				// Skip if target has meaningful content (not empty and not "text")
				shouldSkip := targetTextForCheck != "" && targetTextForCheck != "text"
				// p.Send(logMsg(fmt.Sprintf("Quick mode decision: skip=%t (empty=%t, isText=%t)", shouldSkip, targetTextForCheck == "", targetTextForCheck == "text")))

				if shouldSkip {
					p.Send(logMsg(fmt.Sprintf("Quick mode: skipping row %d", i+1)))
					skippedCount++
					continue
				}
			}
		}

		var translatedText string
		var err error

		// Handle # pattern (existing logic)
		if strings.Contains(text, "#") && strings.Contains(previousText, "#") {
			currentParts := strings.SplitN(text, "#", 2)
			previousParts := strings.SplitN(previousText, "#", 2)

			if len(currentParts) == 2 && len(previousParts) == 2 && currentParts[0] == previousParts[0] {
				translatedPreviousParts := strings.SplitN(previousTranslation, "#", 2)
				if len(translatedPreviousParts) == 2 {
					suffix := strings.TrimSpace(currentParts[1])
					if _, err := strconv.Atoi(suffix); err == nil {
						translatedText = translatedPreviousParts[0] + "#" + suffix
						p.Send(logMsg(fmt.Sprintf("Reused prefix for: %s", text)))
					} else {
						p.Send(logMsg(fmt.Sprintf("Translating suffix: %s", suffix)))
						suffixTranslation, err := translateText(client, suffix, sourceLang, targetLang)
						if err != nil {
							p.Send(logMsg(fmt.Sprintf("ERROR: %v", err)))
							translatedText = text
						} else {
							translatedText = translatedPreviousParts[0] + "#" + suffixTranslation
						}
					}
					goto saveAndContinue
				}
			}
		}

		// Handle underscore + number pattern (new logic)
		if hasUnderscoreNumberPattern(text) && hasUnderscoreNumberPattern(previousText) {
			currentBase, currentSuffix := extractBaseAndSuffix(text)
			previousBase, _ := extractBaseAndSuffix(previousText)

			if currentBase == previousBase {
				translatedPreviousBase, _ := extractBaseAndSuffix(previousTranslation)
				if _, err := strconv.Atoi(currentSuffix); err == nil {
					// Current suffix is a number, reuse the translated base
					translatedText = translatedPreviousBase + "_" + currentSuffix
					p.Send(logMsg(fmt.Sprintf("Reused base for: %s", text)))
					goto saveAndContinue
				} else {
					// Current suffix is not a number, translate it
					p.Send(logMsg(fmt.Sprintf("Translating suffix: %s", currentSuffix)))
					suffixTranslation, err := translateText(client, currentSuffix, sourceLang, targetLang)
					if err != nil {
						p.Send(logMsg(fmt.Sprintf("ERROR: %v", err)))
						translatedText = text
					} else {
						translatedText = translatedPreviousBase + "_" + suffixTranslation
					}
					goto saveAndContinue
				}
			}
		}

		p.Send(logMsg(fmt.Sprintf("Translating: %s", text)))
		translatedText, err = translateText(client, text, sourceLang, targetLang)
		if err != nil {
			p.Send(logMsg(fmt.Sprintf("ERROR: %v", err)))
			continue
		}

	saveAndContinue:
		cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
		f.SetCellValue(sheetName, cell, translatedText)

		previousText = text
		previousTranslation = translatedText
		time.Sleep(50 * time.Millisecond) // Rate limit and slow down for UI
	}
}
