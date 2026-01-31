package appenv

import (
	"context"
	"os"
	"strings"
	"testing"
)

func setupTestEnv(t *testing.T) string {
	testContent := `Var_01='Value'
    Var_02='Value'
Var_03  ='Value'
    Var_04  ='Value'
Var_05=  'Value'
Var_06='Value'# Comment # kljkl
    Var_07='Value' # Comment
Var_08  ='Value' # Comment
    Var_09  ='Value' # Comment
Var_10=  'Value' # Comment
Var_11=  Value# Not a Comment
Var_12=  '#Value' # Comment
Var_13=  #Value# Not a Comment
Var_14=  'Va#lue' # Comment
Var_15=  Va# lue# Not a Comment
Var_16=  Va# lue # Comment
`
	tmpfile, err := os.CreateTemp("", "test.env")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmpfile.Write([]byte(testContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}
	return tmpfile.Name()
}

func TestGet(t *testing.T) {
	tmpFile := setupTestEnv(t)
	defer os.Remove(tmpFile)

	tests := []struct {
		key      string
		expected string
	}{
		{"Var_01", "Value"},
		{"Var_11", "Value# Not a Comment"},
		{"Var_16", "Va# lue"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, err := Get(tt.key, tmpFile)
			if err != nil {
				t.Errorf("Get(%s) error: %v", tt.key, err)
			}
			if val != tt.expected {
				t.Errorf("Get(%s) = %q, want %q", tt.key, val, tt.expected)
			}
		})
	}
}

func TestGetLiteral(t *testing.T) {
	tmpFile := setupTestEnv(t)
	defer os.Remove(tmpFile)

	tests := []struct {
		key      string
		expected string
	}{
		{"Var_01", "'Value'"},
		{"Var_05", "  'Value'"},
		{"Var_10", "  'Value' # Comment"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, err := GetLiteral(tt.key, tmpFile)
			if err != nil {
				t.Errorf("GetLiteral(%s) error: %v", tt.key, err)
			}
			if val != tt.expected {
				t.Errorf("GetLiteral(%s) = %q, want %q", tt.key, val, tt.expected)
			}
		})
	}
}

func TestGetLine(t *testing.T) {
	tmpFile := setupTestEnv(t)
	defer os.Remove(tmpFile)

	tests := []struct {
		key      string
		expected string
	}{
		{"Var_01", "Var_01='Value'"},
		{"Var_02", "    Var_02='Value'"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			val, err := GetLine(tt.key, tmpFile)
			if err != nil {
				t.Errorf("GetLine(%s) error: %v", tt.key, err)
			}
			if val != tt.expected {
				t.Errorf("GetLine(%s) = %q, want %q", tt.key, val, tt.expected)
			}
		})
	}
}

func TestMergeNewOnly(t *testing.T) {
	ctx := context.Background()
	sourceContent := `VAR_A='SourceA'
VAR_B='SourceB'
VAR_C='SourceC'
`
	targetContent := `VAR_A='TargetA'
VAR_D='TargetD'
`
	src, _ := os.CreateTemp("", "src.env")
	defer os.Remove(src.Name())
	os.WriteFile(src.Name(), []byte(sourceContent), 0644)

	tgt, _ := os.CreateTemp("", "tgt.env")
	defer os.Remove(tgt.Name())
	os.WriteFile(tgt.Name(), []byte(targetContent), 0644)

	added, err := MergeNewOnly(ctx, tgt.Name(), src.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Should have added VAR_B and VAR_C
	if len(added) != 2 {
		t.Errorf("Expected 2 added lines, got %d", len(added))
	}

	finalContent, _ := os.ReadFile(tgt.Name())
	finalStr := string(finalContent)

	if !strings.Contains(finalStr, "VAR_B='SourceB'") {
		t.Error("Target missing VAR_B")
	}
	if !strings.Contains(finalStr, "VAR_C='SourceC'") {
		t.Error("Target missing VAR_C")
	}
	if strings.Contains(finalStr, "VAR_A='SourceA'") {
		t.Error("Target should NOT have overwritten VAR_A")
	}
}

func TestGetLineRegex(t *testing.T) {
	testContent := `VAR_A='Val'
VAR_B='Val'
OTHER='Val'
`
	tmp, _ := os.CreateTemp("", "regex.env")
	defer os.Remove(tmp.Name())
	os.WriteFile(tmp.Name(), []byte(testContent), 0644)

	lines, err := GetLineRegex("VAR_.*", tmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(lines) != 2 {
		t.Errorf("Expected 2 matching lines, got %d", len(lines))
	}
}
