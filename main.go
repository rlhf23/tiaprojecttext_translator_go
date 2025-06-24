package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/xuri/excelize/v2"
)

func main() {
	// Check for OpenAI API Key
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("Error: OPENAI_API_KEY environment variable is not set")
	}

	// List .xlsx files
	files, err := filepath.Glob("*.xlsx")
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	var validFiles []string
	for _, file := range files {
		if !strings.HasPrefix(file, "translated") {
			validFiles = append(validFiles, file)
		}
	}

	if len(validFiles) == 0 {
		log.Fatal("No .xlsx files found to translate.")
	}

	fmt.Println("Please select a file to translate:")
	for i, file := range validFiles {
		fmt.Printf("%d: %s\n", i+1, file)
	}

	// Get user input for file selection
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the number of the file: ")
	input, _ := reader.ReadString('\n')
	fileIndex, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || fileIndex < 1 || fileIndex > len(validFiles) {
		log.Fatal("Invalid selection.")
	}
	fileName := validFiles[fileIndex-1]
	fmt.Printf("You selected %s\n", fileName)

	// Open the Excel file
	f, err := excelize.OpenFile(fileName)
	if err != nil {
		log.Fatalf("Error reading Excel file: %v", err)
	}
	defer f.Close()

	// Get the first sheet name
	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Fatalf("Failed to get rows: %v", err)
	}

	if len(rows) == 0 {
		log.Fatal("The selected Excel file is empty.")
	}

	headers := rows[0]

	// Get source language column
	fmt.Println("\nPlease select the source language column:")
	for i, header := range headers {
		if i >= 4 && !strings.HasPrefix(strings.ToLower(header), "ref") {
			fmt.Printf("%d: %s\n", i+1, header)
		}
	}
	fmt.Printf("Enter the number of the source language column ([6]: %s): ", headers[5])
	input, _ = reader.ReadString('\n')
	sourceIndex := 6
	if strings.TrimSpace(input) != "" {
		sourceIndex, err = strconv.Atoi(strings.TrimSpace(input))
		if err != nil || sourceIndex < 1 || sourceIndex > len(headers) {
			log.Fatal("Invalid selection for source language column.")
		}
	}
	sourceLanguageColumn := headers[sourceIndex-1]
	fmt.Printf("Source language column: %s\n", sourceLanguageColumn)

	// Get target language column
	fmt.Println("\nPlease select the target language column:")
	for i, header := range headers {
		if i >= 4 && !strings.HasPrefix(strings.ToLower(header), "ref") {
			fmt.Printf("%d: %s\n", i+1, header)
		}
	}
	fmt.Printf("Enter the number of the target language column ([7]: %s): ", headers[6])
	input, _ = reader.ReadString('\n')
	targetIndex := 7
	if strings.TrimSpace(input) != "" {
		targetIndex, err = strconv.Atoi(strings.TrimSpace(input))
		if err != nil || targetIndex < 1 || targetIndex > len(headers) {
			log.Fatal("Invalid selection for target language column.")
		}
	}
	targetLanguageColumn := headers[targetIndex-1]
	fmt.Printf("Target language column: %s\n", targetLanguageColumn)

	// Translate and update the sheet
	iterateAndTranslate(f, sheetName, rows, sourceIndex-1, targetIndex-1, sourceLanguageColumn, targetLanguageColumn)

	// Save the new file
	newFileName := "translated-" + fileName
	if err := f.SaveAs(newFileName); err != nil {
		log.Fatalf("Failed to save file: %v", err)
	}
	fmt.Printf("\nTranslation complete. File saved as %s\n", newFileName)
}

// Translation function using OpenAI's chat model
func translateText(client *openai.Client, text, sourceLanguage, targetLanguage string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: fmt.Sprintf("You will be provided with a sentence in %s, and your task is to translate it into %s. These are messages concerning industrial machines. Right means the direction right. AC means AC motor.", sourceLanguage, targetLanguage),
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: text,
				},
			},
			Temperature: 0,
			MaxTokens:   60,
		},
	)

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("translation request timed out")
		}
		return "", fmt.Errorf("error during translation: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

var meaninglessAlarmRegex = regexp.MustCompile(`(?i)^alarm\s+\d+:\s*$`) // For alarms like "Alarm 16: "

// isPlaceholder checks if a string is a placeholder that should be copied, not translated.
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

// Iterate over the rows of the Excel file
func iterateAndTranslate(f *excelize.File, sheetName string, rows [][]string, sourceIndex, targetIndex int, sourceLang, targetLang string) {
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	var previousText, previousTranslation string

	for i, row := range rows {
		if i == 0 { // Skip header row
			continue
		}

		if len(row) <= sourceIndex {
			continue
		}

		text := strings.TrimSpace(row[sourceIndex])

		// Basic validation: skip empty, short, or purely numeric strings
		if len(text) < 3 {
			continue
		}
		if _, err := strconv.Atoi(text); err == nil {
			continue
		}

		// Handle placeholders: copy them directly without translation
		if isPlaceholder(text) {
			fmt.Printf("Row %d: Copying placeholder '%s'\n", i+1, text)
			cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
			f.SetCellValue(sheetName, cell, text)
			continue
		}

		var translatedText string
		var err error

		// Smart reuse for texts with a "#" separator
		if strings.Contains(text, "#") && strings.Contains(previousText, "#") {
			currentParts := strings.SplitN(text, "#", 2)
			previousParts := strings.SplitN(previousText, "#", 2)

			// If the prefix matches, reuse the translated prefix
			if len(currentParts) == 2 && len(previousParts) == 2 && currentParts[0] == previousParts[0] {
				translatedPreviousParts := strings.SplitN(previousTranslation, "#", 2)
				if len(translatedPreviousParts) == 2 {
					suffix := strings.TrimSpace(currentParts[1])
					// If the suffix is just a number, don't translate it.
					if _, err := strconv.Atoi(suffix); err == nil {
						translatedText = translatedPreviousParts[0] + "#" + suffix
						fmt.Printf("Row %d: Reusing prefix and appending number suffix for '%s'. Result: '%s'\n", i+1, text, translatedText)
					} else {
						// Otherwise, translate the suffix.
						fmt.Printf("Row %d: Reusing prefix for '%s'. Translating suffix '%s'... ", i+1, text, suffix)
						suffixTranslation, err := translateText(client, suffix, sourceLang, targetLang)
						if err != nil {
							fmt.Printf("Error: %v\n", err)
							translatedText = text // Fallback to original text on error
						} else {
							translatedText = translatedPreviousParts[0] + "#" + suffixTranslation
							fmt.Printf("Result: '%s'\n", translatedText)
						}
					}
					goto saveAndContinue
				}
			}
		}

		// Full translation for new or non-matching texts
		fmt.Printf("Row %d: Translating '%s' from %s to %s... ", i+1, text, sourceLang, targetLang)
		translatedText, err = translateText(client, text, sourceLang, targetLang)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue // Skip saving on error
		}
		fmt.Printf("Result: '%s'\n", translatedText)

		saveAndContinue:
		// Save the translated text
		cell, _ := excelize.CoordinatesToCellName(targetIndex+1, i+1)
		f.SetCellValue(sheetName, cell, translatedText)

		// Update history for the next iteration
		previousText = text
		previousTranslation = translatedText
	}
}
