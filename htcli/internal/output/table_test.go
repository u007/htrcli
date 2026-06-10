package output

import (
	"strings"
	"testing"
)

func TestNewTable(t *testing.T) {
	table := NewTable("Name", "Age")
	if len(table.Headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(table.Headers))
	}
}

func TestAddRow(t *testing.T) {
	table := NewTable("Name", "Age")
	table.AddRow("Alice", "30")
	table.AddRow("Bob", "25")

	if len(table.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(table.Rows))
	}
}

func TestTableString(t *testing.T) {
	table := NewTable("ID", "Name", "Active")
	table.AddRow("1", "Example", "yes")
	table.AddRow("2", "Google", "no")

	output := table.String()

	if !strings.Contains(output, "ID") {
		t.Error("expected output to contain header 'ID'")
	}
	if !strings.Contains(output, "Name") {
		t.Error("expected output to contain header 'Name'")
	}
	if !strings.Contains(output, "Example") {
		t.Error("expected output to contain 'Example'")
	}
	if !strings.Contains(output, "Google") {
		t.Error("expected output to contain 'Google'")
	}
	if !strings.Contains(output, "yes") {
		t.Error("expected output to contain 'yes'")
	}
	if !strings.Contains(output, "no") {
		t.Error("expected output to contain 'no'")
	}
}

func TestTableEmptyHeaders(t *testing.T) {
	table := &Table{}
	output := table.String()
	if output != "" {
		t.Errorf("expected empty string for empty headers, got %q", output)
	}
}

func TestTableColumnWidths(t *testing.T) {
	table := NewTable("A", "B")
	table.AddRow("short", "a much longer value")
	table.AddRow("medium", "b")

	output := table.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have header + separator + 2 rows = 4 lines
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d: %v", len(lines), lines)
	}

	// Verify header line has consistent column alignment
	if len(lines[0]) != len(lines[2]) {
		t.Errorf("header and first data row should have same length")
	}
}
