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

func TestHasEmbeddedRefs(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"Calibration weight: /*N:6 {#1.Calibration_weight} NOFILL DP:3*/", true},
		{"Stable Weight:", false},
		{"/*S:0 {Scale\\kg_pounds_string}*/", true},
		{"Some text /*N:6 {#1.Value}*/ more text", true},
		{"No refs here", false},
		{"", false},
		{"**REF:704**", false},
		{"Weight: /*N:6 #1.Weight*/ /*S:0 {Unit}*/", true},
	}

	for _, tc := range testCases {
		result := hasEmbeddedRefs(tc.input)
		if result != tc.expected {
			t.Errorf("hasEmbeddedRefs(%q) = %t; expected %t", tc.input, result, tc.expected)
		}
	}
}

func TestExtractEmbeddedRefs(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expected    []string
		expectedLen int
	}{
		{
			name:        "Single ref",
			input:       "Calibration weight: /*N:6 {#1.Calibration_weight} NOFILL DP:3*/",
			expected:    []string{"/*N:6 {#1.Calibration_weight} NOFILL DP:3*/"},
			expectedLen: 1,
		},
		{
			name:        "Multiple refs",
			input:       "Calibration weight: /*N:6 {#1.Calibration_weight}*/ /*S:0 {Scale\\kg_pounds_string}*/",
			expected:    []string{"/*N:6 {#1.Calibration_weight}*/", "/*S:0 {Scale\\kg_pounds_string}*/"},
			expectedLen: 2,
		},
		{
			name:        "No refs",
			input:       "Stable Weight:",
			expected:    []string{},
			expectedLen: 0,
		},
		{
			name:        "Ref at end",
			input:       "Max expected weight: /*N:6 #1.Max_expected NOFILL DP:3*/",
			expected:    []string{"/*N:6 #1.Max_expected NOFILL DP:3*/"},
			expectedLen: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractEmbeddedRefs(tc.input)
			if len(result) != tc.expectedLen {
				t.Errorf("extractEmbeddedRefs(%q) returned %d refs; expected %d", tc.input, len(result), tc.expectedLen)
			}
			for i, ref := range result {
				if i < len(tc.expected) && ref != tc.expected[i] {
					t.Errorf("extractEmbeddedRefs(%q)[%d] = %q; expected %q", tc.input, i, ref, tc.expected[i])
				}
			}
		})
	}
}

func TestSplitTextByRefs(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Text with single ref",
			input:    "Calibration weight: /*N:6 {#1.Calibration_weight} NOFILL DP:3*/",
			expected: []string{"Calibration weight: ", "/*N:6 {#1.Calibration_weight} NOFILL DP:3*/", ""},
		},
		{
			name:     "Text with two refs",
			input:    "Weight: /*N:6 {#1.Value}*/ /*S:0 {Unit}*/",
			expected: []string{"Weight: ", "/*N:6 {#1.Value}*/", " ", "/*S:0 {Unit}*/", ""},
		},
		{
			name:     "No refs",
			input:    "Stable Weight:",
			expected: []string{"Stable Weight:"},
		},
		{
			name:     "Ref at start",
			input:    "/*N:6 {#1.Value}*/ total",
			expected: []string{"", "/*N:6 {#1.Value}*/", " total"},
		},
		{
			name:     "Only refs",
			input:    "/*N:6 {#1.Value}*/",
			expected: []string{"", "/*N:6 {#1.Value}*/", ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := splitTextByRefs(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("splitTextByRefs(%q) returned %d segments; expected %d: %v", tc.input, len(result), len(tc.expected), result)
				return
			}
			for i, seg := range result {
				if seg != tc.expected[i] {
					t.Errorf("splitTextByRefs(%q)[%d] = %q; expected %q", tc.input, i, seg, tc.expected[i])
				}
			}
		})
	}
}

func TestReassembleWithRefs(t *testing.T) {
	testCases := []struct {
		name       string
		translated []string
		expected   string
	}{
		{
			name:       "Single ref preserved",
			translated: []string{"Peso de calibración: ", "/*N:6 {#1.Calibration_weight} NOFILL DP:3*/"},
			expected:   "Peso de calibración: /*N:6 {#1.Calibration_weight} NOFILL DP:3*/",
		},
		{
			name:       "Multiple refs preserved",
			translated: []string{"Weight: ", "/*N:6 {#1.Value}*/", " ", "/*S:0 {Unit}*/"},
			expected:   "Weight: /*N:6 {#1.Value}*/ /*S:0 {Unit}*/",
		},
		{
			name:       "No refs",
			translated: []string{"Stable Weight:"},
			expected:   "Stable Weight:",
		},
		{
			name:       "Ref at start",
			translated: []string{"", "/*N:6 {#1.Value}*/", " peso total"},
			expected:   "/*N:6 {#1.Value}*/ peso total",
		},
		{
			name:       "Only refs",
			translated: []string{"", "/*N:6 {#1.Value}*/", ""},
			expected:   "/*N:6 {#1.Value}*/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := reassembleWithRefs(tc.translated)
			if result != tc.expected {
				t.Errorf("reassembleWithRefs(%v) = %q; expected %q", tc.translated, result, tc.expected)
			}
		})
	}
}
