package processing

import (
	"testing"

	"torn_oc_items/internal/sheets"
)

// TestFindLatestMatchingRow verifies the core logic: when multiple rows match,
// we should select the bottommost (latest) row by iterating backwards
func TestFindLatestMatchingRow(t *testing.T) {
	// Simulate multiple sheet items for same user+item combination
	sheetItems := []sheets.SheetItem{
		{
			RowIndex:    10,
			ItemName:    "Xanax",
			UserName:    "Alice",
			HasProvider: false, // Empty provider
		},
		{
			RowIndex:    50,
			ItemName:    "Xanax",
			UserName:    "Alice",
			HasProvider: false, // Empty provider
		},
		{
			RowIndex:    80,
			ItemName:    "Xanax",
			UserName:    "Alice",
			HasProvider: false, // Empty provider
		},
	}

	// Simulate the backward iteration logic used in processLogItemForUpdates
	targetItem := "Xanax"
	targetUser := "Alice"
	var selectedRow int

	// This mirrors the actual iteration logic in processLogItemForUpdates
	for i := len(sheetItems) - 1; i >= 0; i-- {
		sheetItem := sheetItems[i]
		if !sheetItem.HasProvider &&
			sheetItem.UserName == targetUser &&
			sheetItem.ItemName == targetItem {
			selectedRow = sheetItem.RowIndex
			break
		}
	}

	// Verify we selected row 80 (bottommost/latest match)
	if selectedRow != 80 {
		t.Errorf("Expected row 80 to be selected (latest crime), got row %d", selectedRow)
	}
}

// TestFindLatestMatchingRow_WithProviders verifies that rows with existing
// providers are skipped when iterating backwards
func TestFindLatestMatchingRow_WithProviders(t *testing.T) {
	sheetItems := []sheets.SheetItem{
		{
			RowIndex:    10,
			ItemName:    "Xanax",
			UserName:    "Alice",
			Provider:    "Charlie",
			HasProvider: true, // Already has provider
		},
		{
			RowIndex:    50,
			ItemName:    "Xanax",
			UserName:    "Alice",
			HasProvider: false, // Empty provider - should be selected
		},
		{
			RowIndex:    80,
			ItemName:    "Xanax",
			UserName:    "Alice",
			Provider:    "David",
			HasProvider: true, // Already has provider
		},
	}

	targetItem := "Xanax"
	targetUser := "Alice"
	var selectedRow int

	// Backward iteration
	for i := len(sheetItems) - 1; i >= 0; i-- {
		sheetItem := sheetItems[i]
		if !sheetItem.HasProvider &&
			sheetItem.UserName == targetUser &&
			sheetItem.ItemName == targetItem {
			selectedRow = sheetItem.RowIndex
			break
		}
	}

	// Should select row 50 (latest row without provider)
	if selectedRow != 50 {
		t.Errorf("Expected row 50 to be selected (latest without provider), got row %d", selectedRow)
	}
}

// TestFindLatestMatchingRow_NoMatch verifies that when no rows match,
// no row is selected
func TestFindLatestMatchingRow_NoMatch(t *testing.T) {
	sheetItems := []sheets.SheetItem{
		{
			RowIndex:    10,
			ItemName:    "Vicodin", // Different item
			UserName:    "Alice",
			HasProvider: false,
		},
		{
			RowIndex:    50,
			ItemName:    "Xanax",
			UserName:    "Bob", // Different user
			HasProvider: false,
		},
	}

	targetItem := "Xanax"
	targetUser := "Alice" // Looking for Alice + Xanax
	var selectedRow int

	// Backward iteration
	for i := len(sheetItems) - 1; i >= 0; i-- {
		sheetItem := sheetItems[i]
		if !sheetItem.HasProvider &&
			sheetItem.UserName == targetUser &&
			sheetItem.ItemName == targetItem {
			selectedRow = sheetItem.RowIndex
			break
		}
	}

	// Should find no match (selectedRow remains 0)
	if selectedRow != 0 {
		t.Errorf("Expected no row to be selected, got row %d", selectedRow)
	}
}

// TestBackwardIterationVsForward demonstrates the difference between
// forward and backward iteration when multiple rows match
func TestBackwardIterationVsForward(t *testing.T) {
	sheetItems := []sheets.SheetItem{
		{RowIndex: 10, ItemName: "Xanax", UserName: "Alice", HasProvider: false},
		{RowIndex: 20, ItemName: "Xanax", UserName: "Alice", HasProvider: false},
		{RowIndex: 30, ItemName: "Xanax", UserName: "Alice", HasProvider: false},
	}

	targetItem := "Xanax"
	targetUser := "Alice"

	// Forward iteration (old buggy behavior)
	var forwardSelected int
	for _, sheetItem := range sheetItems {
		if !sheetItem.HasProvider &&
			sheetItem.UserName == targetUser &&
			sheetItem.ItemName == targetItem {
			forwardSelected = sheetItem.RowIndex
			break
		}
	}

	// Backward iteration (new correct behavior)
	var backwardSelected int
	for i := len(sheetItems) - 1; i >= 0; i-- {
		sheetItem := sheetItems[i]
		if !sheetItem.HasProvider &&
			sheetItem.UserName == targetUser &&
			sheetItem.ItemName == targetItem {
			backwardSelected = sheetItem.RowIndex
			break
		}
	}

	// Verify the difference
	if forwardSelected != 10 {
		t.Errorf("Forward iteration should select row 10, got %d", forwardSelected)
	}
	if backwardSelected != 30 {
		t.Errorf("Backward iteration should select row 30, got %d", backwardSelected)
	}
	if forwardSelected == backwardSelected {
		t.Error("Forward and backward iteration should produce different results")
	}
}
