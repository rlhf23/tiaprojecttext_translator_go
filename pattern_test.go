package main

import (
	"testing"
)

func TestHasUnderscoreNumberPattern(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"Discrete_alarm_66", true},
		{"Discrete_alarm_67", true},
		{"DQ16x24VDC/0.5AST_21", true},
		{"DQ16x24VDC/0.5AST_22", true},
		{"SomeOtherText", false},
		{"NoNumber_Here", false},
		{"Single_123", true},
		{"Multiple_underscores_in_text_45", true},
		{"", false},
		{"_", false},
		{"_123", true},
		{"text_", false},
	}

	for _, tc := range testCases {
		result := hasUnderscoreNumberPattern(tc.input)
		if result != tc.expected {
			t.Errorf("hasUnderscoreNumberPattern(%q) = %t; expected %t", tc.input, result, tc.expected)
		}
	}
}

func TestExtractBaseAndSuffix(t *testing.T) {
	testCases := []struct {
		input          string
		expectedBase   string
		expectedSuffix string
	}{
		{"Discrete_alarm_66", "Discrete_alarm", "66"},
		{"Discrete_alarm_67", "Discrete_alarm", "67"},
		{"DQ16x24VDC/0.5AST_21", "DQ16x24VDC/0.5AST", "21"},
		{"DQ16x24VDC/0.5AST_22", "DQ16x24VDC/0.5AST", "22"},
		{"Single_123", "Single", "123"},
		{"Multiple_underscores_in_text_45", "Multiple_underscores_in_text", "45"},
		{"NoUnderscore", "NoUnderscore", ""},
		{"", "", ""},
		{"_", "", ""},
		{"_123", "", "123"},
		{"text_", "text", ""},
	}

	for _, tc := range testCases {
		base, suffix := extractBaseAndSuffix(tc.input)
		if base != tc.expectedBase {
			t.Errorf("extractBaseAndSuffix(%q) base = %q; expected %q", tc.input, base, tc.expectedBase)
		}
		if suffix != tc.expectedSuffix {
			t.Errorf("extractBaseAndSuffix(%q) suffix = %q; expected %q", tc.input, suffix, tc.expectedSuffix)
		}
	}
}

func TestHasSpaceNumberPattern(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"AR: Warning 82", true},
		{"AR: Warning 83", true},
		{"AR: Warning 9", true},
		{"Some text 1", true},
		{"Multiple words here 999", true},
		{"AR: Warning", false},
		{"AR: Warning ABC", false},
		{"AR: Warning 9 Spare emergency stop button activated", false},
		{"123", false},
		{"", false},
		{"SingleWord", false},
		{"Text with space but no number", false},
	}

	for _, tc := range testCases {
		result := hasSpaceNumberPattern(tc.input)
		if result != tc.expected {
			t.Errorf("hasSpaceNumberPattern(%q) = %t; expected %t", tc.input, result, tc.expected)
		}
	}
}

func TestExtractSpaceBaseAndSuffix(t *testing.T) {
	testCases := []struct {
		input          string
		expectedBase   string
		expectedSuffix string
	}{
		{"AR: Warning 82", "AR: Warning", "82"},
		{"AR: Warning 83", "AR: Warning", "83"},
		{"AR: Warning 9", "AR: Warning", "9"},
		{"Some text 123", "Some text", "123"},
		{"", "", ""},
	}

	for _, tc := range testCases {
		base, suffix := extractSpaceBaseAndSuffix(tc.input)
		if base != tc.expectedBase {
			t.Errorf("extractSpaceBaseAndSuffix(%q) base = %q; expected %q", tc.input, base, tc.expectedBase)
		}
		if suffix != tc.expectedSuffix {
			t.Errorf("extractSpaceBaseAndSuffix(%q) suffix = %q; expected %q", tc.input, suffix, tc.expectedSuffix)
		}
	}
}

func TestShouldReuseTranslation(t *testing.T) {
	testCases := []struct {
		name           string
		currentText    string
		previousText   string
		expectedReuse  bool
		expectedBase   string
		expectedSuffix string
		expectedDelim  string
	}{
		{
			name:           "Same base underscore pattern",
			currentText:    "Discrete_alarm_67",
			previousText:   "Discrete_alarm_66",
			expectedReuse:  true,
			expectedBase:   "Discrete_alarm",
			expectedSuffix: "67",
			expectedDelim:  "_",
		},
		{
			name:           "Different base underscore pattern",
			currentText:    "Discrete_alarm_67",
			previousText:   "Other_alarm_66",
			expectedReuse:  false,
			expectedBase:   "",
			expectedSuffix: "",
			expectedDelim:  "",
		},
		{
			name:           "Same base space pattern",
			currentText:    "AR: Warning 83",
			previousText:   "AR: Warning 82",
			expectedReuse:  true,
			expectedBase:   "AR: Warning",
			expectedSuffix: "83",
			expectedDelim:  " ",
		},
		{
			name:           "Different base space pattern",
			currentText:    "AR: Warning 83",
			previousText:   "AR: Error 82",
			expectedReuse:  false,
			expectedBase:   "",
			expectedSuffix: "",
			expectedDelim:  "",
		},
		{
			name:           "Mixed patterns - underscore current, space previous",
			currentText:    "Discrete_alarm_67",
			previousText:   "AR: Warning 82",
			expectedReuse:  false,
			expectedBase:   "",
			expectedSuffix: "",
			expectedDelim:  "",
		},
		{
			name:           "Mixed patterns - space current, underscore previous",
			currentText:    "AR: Warning 83",
			previousText:   "Discrete_alarm_66",
			expectedReuse:  false,
			expectedBase:   "",
			expectedSuffix: "",
			expectedDelim:  "",
		},
		{
			name:           "Same hash pattern",
			currentText:    "Warning#83",
			previousText:   "Warning#82",
			expectedReuse:  true,
			expectedBase:   "Warning",
			expectedSuffix: "83",
			expectedDelim:  "#",
		},
		{
			name:           "Different hash pattern",
			currentText:    "Warning#83",
			previousText:   "Error#82",
			expectedReuse:  false,
			expectedBase:   "",
			expectedSuffix: "",
			expectedDelim:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldReuse, base, suffix, delim := shouldReuseTranslation(tc.currentText, tc.previousText)
			if shouldReuse != tc.expectedReuse {
				t.Errorf("shouldReuseTranslation(%q, %q) reuse = %t; expected %t", tc.currentText, tc.previousText, shouldReuse, tc.expectedReuse)
			}
			if shouldReuse && base != tc.expectedBase {
				t.Errorf("shouldReuseTranslation(%q, %q) base = %q; expected %q", tc.currentText, tc.previousText, base, tc.expectedBase)
			}
			if shouldReuse && suffix != tc.expectedSuffix {
				t.Errorf("shouldReuseTranslation(%q, %q) suffix = %q; expected %q", tc.currentText, tc.previousText, suffix, tc.expectedSuffix)
			}
			if shouldReuse && delim != tc.expectedDelim {
				t.Errorf("shouldReuseTranslation(%q, %q) delim = %q; expected %q", tc.currentText, tc.previousText, delim, tc.expectedDelim)
			}
		})
	}
}
