package main

import (
	"strings"
	"testing"
)

func TestIsVisualSeparator(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		// Should be true - mostly separators
		{"---------------------------------------------", true},
		{"=============================================", true},
		{"_____________________________________________", true},
		{"*********************************************", true},
		{".............................................", true},
		{"-----", true},
		{"=====", true},
		{"_____", true},

		// Should be false - too short
		{"-", false},
		{"--", false},
		{"==", false},
		{"__", false},

		// Should be false - not mostly separators
		{"Hello world", false},
		{"Some-text-with-dashes", false},
		{"Text with underscores_here", false},
		{"123-456-789", false},
		{"A-B-C-D-E", false},

		// Edge cases - mixed but mostly separators
		{"---------------------------------------------text", true}, // still more than 80% separators
		{"text---------------------------------------------", true}, // still more than 80% separators
		{"-----text-----", false},                                   // 10/13 = 0.77 < 0.8
	}

	for _, tc := range testCases {
		result := isVisualSeparator(tc.input)
		if result != tc.expected {
			t.Errorf("isVisualSeparator(%q) = %t; expected %t", tc.input, result, tc.expected)
		}
	}
}

func TestQuickModeLogic(t *testing.T) {
	// Test the logic that determines whether to translate in quick mode
	testCases := []struct {
		targetText      string
		shouldTranslate bool
		description     string
	}{
		{"", true, "Empty target should translate"},
		{"Text", true, "Target with 'Text' should translate"},
		{" text ", true, "Target with spaces around 'Text' should translate"},
		{"\"Text\"", true, "Target with quoted 'Text' should translate"},
		{"\" text \"", false, "Target with quoted 'Text' and spaces should NOT translate"},
		{"TEXT", true, "Target with uppercase 'TEXT' should translate"},
		{"text", true, "Target with lowercase 'text' should translate"},
		{"\"TEXT\"", true, "Target with quoted uppercase 'TEXT' should translate"},

		{"Actual translation", false, "Target with actual translation should not translate"},
		{"Some text", false, "Target with some text should not translate"},
		{"Translated content", false, "Target with content should not translate"},
	}

	for _, tc := range testCases {
		// Simulate the actual logic from the code
		targetText := strings.TrimSpace(tc.targetText)
		targetTextForCheck := strings.ToLower(strings.Trim(targetText, `"`))

		shouldSkip := targetTextForCheck != "" && targetTextForCheck != "text"
		shouldTranslate := !shouldSkip

		if shouldTranslate != tc.shouldTranslate {
			t.Errorf("%s: target=%q -> shouldTranslate=%t; expected %t (processed=%q)",
				tc.description, tc.targetText, shouldTranslate, tc.shouldTranslate, targetTextForCheck)
		}
	}
}
