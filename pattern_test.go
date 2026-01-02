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
