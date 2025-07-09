package app

// SheetRow represents a row read from the spreadsheet
type SheetRow struct {
	RowIndex int
	CrimeURL string
	ItemName string
	UserName string
	Provider string // Column B
	DateTime string // Column D
}

// SheetRowUpdate represents an update to be made to a sheet row
type SheetRowUpdate struct {
	RowIndex    int
	Provider    string
	DateTime    string
	MarketValue float64
}

// SheetItem represents a parsed item from the spreadsheet
type SheetItem struct {
	RowIndex    int
	CrimeURL    string
	ItemName    string
	UserName    string
	Provider    string
	HasProvider bool
}
