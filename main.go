package main

import (
	"context"
	"fmt"
	"log"
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
	percent    float64
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
		if len(m.logMessages) > 5 {
			m.logMessages = m.logMessages[1:]
		}
		return m, nil

	case doneMsg:
		m.done = true
		return m, tea.Quit

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

func main() {
	// ///////////////////
	// 1. GET USER INPUT
	// ///////////////////
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("Error: OPENAI_API_KEY environment variable not set.")
	}

	files, err := filepath.Glob("*.xlsx")
	if err != nil {
		log.Fatalf("Error finding .xlsx files: %v", err)
	}

	var filteredFiles []string
	for _, file := range files {
		if !strings.HasPrefix(file, "translated-") {
			filteredFiles = append(filteredFiles, file)
		}
	}

	if len(filteredFiles) == 0 {
		log.Fatal("No .xlsx files found to translate.")
	}

	var fileName string
	var sourceLangIndex, targetLangIndex int

	fileOptions := make([]huh.Option[string], len(filteredFiles))
	for i, f := range filteredFiles {
		fileOptions[i] = huh.NewOption(f, f)
	}

	form := huh.NewForm(
		huh.NewGroup(huh.NewSelect[string]().Title("Select a file to translate").Options(fileOptions...).Value(&fileName)),
	)

	if err := form.Run(); err != nil {
		log.Fatal(err)
	}

	f, err := excelize.OpenFile(fileName)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Fatalf("Error getting rows: %v", err)
	}
	headers := rows[0]
	var colOptions []huh.Option[int]
	for i, h := range headers {
		// Filter out reference columns, which are not useful for translation.
		if !strings.HasPrefix(strings.ToLower(h), "ref=") {
			colOptions = append(colOptions, huh.NewOption(fmt.Sprintf("%s (Col %d)", h, i+1), i))
		}
	}

	langForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().Title("Select Source Language Column").Options(colOptions...).Value(&sourceLangIndex),
			huh.NewSelect[int]().Title("Select Target Language Column").Options(colOptions...).Value(&targetLangIndex),
		),
	)

	if err := langForm.Run(); err != nil {
		log.Fatal(err)
	}

	// ///////////////////
	// 2. RUN TRANSLATION WITH TUI
	// ///////////////////
	m := model{
		progressBar: progress.New(progress.WithDefaultGradient()),
	}
	p := tea.NewProgram(m)

	go iterateAndTranslate(p, f, sheetName, rows, sourceLangIndex, targetLangIndex, headers[sourceLangIndex], headers[targetLangIndex])

	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}

	// ///////////////////
	// 3. SAVE FILE
	// ///////////////////
	newFileName := "translated-" + fileName
	if err := f.SaveAs(newFileName); err != nil {
		log.Fatalf("Error saving new file: %v", err)
	}
	fmt.Println(helpStyle.Render(fmt.Sprintf("\nTranslation saved to %s", newFileName)))
}

func translateText(client *openai.Client, text, sourceLang, targetLang string) (string, error) {
	prompt := fmt.Sprintf("You are a professional translator. Translate the following text from '%s' to '%s'. Do not add any extra conversational text, just provide the translation. If the text is a placeholder or code, return it as is. The text to translate is: \"%s\"", sourceLang, targetLang, text)
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
	return resp.Choices[0].Message.Content, nil
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

func iterateAndTranslate(p *tea.Program, f *excelize.File, sheetName string, rows [][]string, sourceIndex, targetIndex int, sourceLang, targetLang string) {
	defer func() { p.Send(doneMsg{}) }()

	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
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

		if len(text) < 3 || (len(text) > 0 && text[0] == '!') {
			continue
		}
		if _, err := strconv.Atoi(text); err == nil {
			continue
		}

		if isPlaceholder(text) {
			p.Send(logMsg(fmt.Sprintf("Copied placeholder: %s", text)))
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, text)
			time.Sleep(10 * time.Millisecond) // Slow down for UI
			continue
		}

		var translatedText string
		var err error

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
