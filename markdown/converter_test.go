package markdown

import (
	"strings"
	"testing"
)

const sampleTable = `Here is the comparison:

| Tool | Stars | Language |
| --- | --- | --- |
| Foo | 1.2k | Go |
| Bar | 800 | Rust |

That's all.
`

func TestConvertTablesCodeMode(t *testing.T) {
	out := ConvertTables(sampleTable, TableModeCode)

	if !strings.Contains(out, "```") {
		t.Fatalf("code mode should wrap table in fenced block, got:\n%s", out)
	}
	if strings.Contains(out, "| --- |") {
		t.Fatalf("GFM divider row should be replaced, got:\n%s", out)
	}
	if !strings.Contains(out, "Tool") || !strings.Contains(out, "Foo") || !strings.Contains(out, "Bar") {
		t.Fatalf("cells missing in output:\n%s", out)
	}
	// Surrounding prose preserved.
	if !strings.Contains(out, "Here is the comparison:") || !strings.Contains(out, "That's all.") {
		t.Fatalf("surrounding prose lost:\n%s", out)
	}
}

func TestConvertTablesBulletsMode(t *testing.T) {
	out := ConvertTables(sampleTable, TableModeBullets)

	if strings.Contains(out, "|") {
		t.Fatalf("bullets mode should remove pipe characters, got:\n%s", out)
	}
	if !strings.Contains(out, "• Tool: Foo") {
		t.Fatalf("expected '• Tool: Foo' in bullet output, got:\n%s", out)
	}
	if !strings.Contains(out, "• Language: Rust") {
		t.Fatalf("expected '• Language: Rust' in bullet output, got:\n%s", out)
	}
}

func TestConvertTablesOffMode(t *testing.T) {
	out := ConvertTables(sampleTable, TableModeOff)
	if out != sampleTable {
		t.Fatalf("off mode should return input unchanged.\nwant:\n%s\ngot:\n%s", sampleTable, out)
	}
}

func TestConvertTablesPassThroughNonTable(t *testing.T) {
	in := "# Title\n\nJust text, no table here. **bold** _italic_ `code`.\n"
	out := ConvertTables(in, TableModeCode)
	if out != in {
		t.Fatalf("non-table input should be unchanged.\nwant:\n%s\ngot:\n%s", in, out)
	}
}

func TestConvertTablesPreservesCodeBlocks(t *testing.T) {
	in := "Example:\n\n```go\nfunc x() {}\n```\n\nDone.\n"
	out := ConvertTables(in, TableModeCode)
	if out != in {
		t.Fatalf("fenced code blocks must not be touched.\nwant:\n%s\ngot:\n%s", in, out)
	}
}

func TestConvertTablesMultipleTables(t *testing.T) {
	in := "First:\n\n| a | b |\n| --- | --- |\n| 1 | 2 |\n\nSecond:\n\n| x | y |\n| --- | --- |\n| 9 | 8 |\n"
	out := ConvertTables(in, TableModeCode)
	if strings.Count(out, "```") != 4 { // 2 tables × open+close fences
		t.Fatalf("expected two fenced blocks (4 ``` markers), got:\n%s", out)
	}
}

func TestParseMode(t *testing.T) {
	cases := map[string]TableMode{
		"":         TableModeCode,
		"code":     TableModeCode,
		"CODE":     TableModeCode,
		"bullets":  TableModeBullets,
		"Bullets":  TableModeBullets,
		"off":      TableModeOff,
		"none":     TableModeOff,
		"disabled": TableModeOff,
		"junk":     TableModeCode,
	}
	for in, want := range cases {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConvertTablesEmptyInput(t *testing.T) {
	if got := ConvertTables("", TableModeCode); got != "" {
		t.Fatalf("expected empty output for empty input, got %q", got)
	}
}
