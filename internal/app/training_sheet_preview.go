package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type trainingSheetPreviewRequest struct {
	PlannedActivityID string  `json:"plannedActivityId"`
	Feedback          *string `json:"feedback,omitempty"`
	RPE               *int    `json:"rpe"`
	RPESet            bool    `json:"rpeSet"`
}

type trainingSheetPreviewUpdate struct {
	Update            googleValueRangeUpdate
	Section           string
	Label             string
	ReplaceExisting   bool
	RepairZeroClockHR bool
}

type TrainingSheetPreviewChange struct {
	Range         string `json:"range"`
	Section       string `json:"section"`
	Label         string `json:"label"`
	CurrentValue  string `json:"currentValue"`
	ProposedValue string `json:"proposedValue"`
	Status        string `json:"status"`
}

type TrainingSheetPreviewCellStyle struct {
	BackgroundColor     string `json:"backgroundColor,omitempty"`
	TextColor           string `json:"textColor,omitempty"`
	Bold                bool   `json:"bold,omitempty"`
	Italic              bool   `json:"italic,omitempty"`
	FontSize            int    `json:"fontSize,omitempty"`
	HorizontalAlignment string `json:"horizontalAlignment,omitempty"`
	VerticalAlignment   string `json:"verticalAlignment,omitempty"`
	WrapStrategy        string `json:"wrapStrategy,omitempty"`
}

type TrainingSheetPreviewColumn struct {
	Index   int    `json:"index"`
	Label   string `json:"label"`
	WidthPx int    `json:"widthPx,omitempty"`
	Hidden  bool   `json:"hidden,omitempty"`
}

type TrainingSheetPreviewCell struct {
	Ref           string                         `json:"ref"`
	CurrentValue  string                         `json:"currentValue"`
	DisplayValue  string                         `json:"displayValue"`
	ProposedValue string                         `json:"proposedValue,omitempty"`
	Status        string                         `json:"status"`
	Section       string                         `json:"section,omitempty"`
	Label         string                         `json:"label,omitempty"`
	Style         *TrainingSheetPreviewCellStyle `json:"style,omitempty"`
	RowSpan       int                            `json:"rowSpan,omitempty"`
	ColumnSpan    int                            `json:"columnSpan,omitempty"`
}

type TrainingSheetPreviewRow struct {
	Index    int                        `json:"index"`
	HeightPx int                        `json:"heightPx,omitempty"`
	Cells    []TrainingSheetPreviewCell `json:"cells"`
}

type TrainingSheetPreviewGrid struct {
	StartRow            int                          `json:"startRow"`
	EndRow              int                          `json:"endRow"`
	StartColumn         int                          `json:"startColumn"`
	EndColumn           int                          `json:"endColumn"`
	FormattingAvailable bool                         `json:"formattingAvailable"`
	Columns             []TrainingSheetPreviewColumn `json:"columns"`
	Rows                []TrainingSheetPreviewRow    `json:"rows"`
}

type TrainingSheetWritebackPreview struct {
	ActivityID        string                       `json:"activityId"`
	PlannedActivityID string                       `json:"plannedActivityId"`
	SheetTitle        string                       `json:"sheetTitle"`
	SheetURL          string                       `json:"sheetUrl"`
	Fingerprint       string                       `json:"fingerprint"`
	Changes           []TrainingSheetPreviewChange `json:"changes"`
	Grid              TrainingSheetPreviewGrid     `json:"grid"`
	Warnings          []string                     `json:"warnings,omitempty"`
	WriteCount        int                          `json:"writeCount"`
	ConflictCount     int                          `json:"conflictCount"`
}

func (s *TrainingSheetWritebackService) Preview(ctx context.Context, plannedID, activityID string, request trainingSheetPreviewRequest) (TrainingSheetWritebackPreview, error) {
	planned, err := s.store.GetPlannedActivity(ctx, plannedID)
	if err != nil {
		return TrainingSheetWritebackPreview{}, err
	}
	activity, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return TrainingSheetWritebackPreview{}, err
	}
	if err := validatePlannedActivityMatch(activity, planned, activityID); err != nil {
		return TrainingSheetWritebackPreview{}, err
	}
	applyTrainingSheetPreviewDraft(&activity, request)

	status, err := s.auth.Status(ctx)
	if err != nil {
		return TrainingSheetWritebackPreview{}, err
	}
	if !status.WriteReady {
		return TrainingSheetWritebackPreview{}, fmt.Errorf("Google Sheets write access requires reconnecting the Google account")
	}

	updates, warnings := trainingSheetPreviewUpdates(planned, activity)
	region := trainingSheetPreviewRegion(planned)
	gridResponse, err := s.auth.ReadPreviewGrid(ctx, planned.WorkbookID, planned.SheetTitle, region.rangeName)
	formattingAvailable := true
	var previewGrid trainingSheetPreviewGrid
	if err != nil {
		fallback, fallbackErr := s.auth.ReadRanges(ctx, planned.WorkbookID, []string{region.rangeName})
		if fallbackErr != nil {
			return TrainingSheetWritebackPreview{}, err
		}
		previewGrid = trainingSheetPreviewGridFromValues(region, firstPreviewRangeValues(fallback))
		formattingAvailable = false
		warnings = append(warnings, "Sheet formatting was unavailable; showing live values in a simplified grid.")
	} else {
		previewGrid = trainingSheetPreviewGridFromGoogle(region, gridResponse)
	}
	existing := trainingSheetPreviewExistingValues(previewGrid, updates)

	changes := make([]TrainingSheetPreviewChange, 0, len(updates))
	writeCount := 0
	conflictCount := 0
	for index, item := range updates {
		current := trainingSheetPreviewCurrentValue(existing, index)
		proposed := previewValueText(item.Update.Values)
		status := trainingSheetPreviewStatus(item, current, proposed)
		if status == "write" {
			writeCount++
		}
		if status == "conflict" {
			conflictCount++
		}
		changes = append(changes, TrainingSheetPreviewChange{
			Range: item.Update.Range, Section: item.Section, Label: item.Label,
			CurrentValue: current, ProposedValue: proposed, Status: status,
		})
	}
	return TrainingSheetWritebackPreview{
		ActivityID: activityID, PlannedActivityID: plannedID, SheetTitle: planned.SheetTitle,
		SheetURL: planned.SourceURL, Fingerprint: trainingSheetPreviewFingerprint(plannedID, activityID, updates, existing),
		Changes: changes, Grid: trainingSheetPreviewGridResponse(previewGrid, region, planned, changes, formattingAvailable),
		Warnings: warnings, WriteCount: writeCount, ConflictCount: conflictCount,
	}, nil
}

func validatePlannedActivityMatch(activity Activity, planned PlannedActivity, activityID string) error {
	if activity.Source == trainingSheetProvider {
		return errPlannedMatchInvalid
	}
	if planned.MatchedActivityID != "" && planned.MatchedActivityID != activityID {
		return errPlannedMatchConflict
	}
	if planned.MatchedActivityID == "" && planned.Status != "pending" {
		return errPlannedMatchConflict
	}
	return nil
}

func applyTrainingSheetPreviewDraft(activity *Activity, request trainingSheetPreviewRequest) {
	if request.Feedback != nil {
		activity.Feedback = strings.TrimSpace(*request.Feedback)
	}
	if request.RPESet {
		activity.RPE = request.RPE
	}
}

func trainingSheetPreviewUpdates(planned PlannedActivity, activity Activity) ([]trainingSheetPreviewUpdate, []string) {
	updates := make([]trainingSheetPreviewUpdate, 0)
	warnings := make([]string, 0)
	for _, update := range summaryUpdatesForActivity(planned, activity) {
		updates = append(updates, trainingSheetPreviewUpdate{Update: update, Section: "summary", Label: summaryLabelForRange(update.Range)})
	}

	if workoutTableFromPlanned(planned) != nil {
		intervalUpdates, err := intervalUpdatesForPlannedActivity(planned, activity)
		if err != nil {
			warnings = append(warnings, "Structured intervals could not be mapped: "+err.Error())
		} else {
			table := workoutTableFromPlanned(planned)
			heartColumns := map[string]bool{table.Columns[trainingSheetMetricAvgHeart]: true, table.Columns[trainingSheetMetricMaxHeart]: true}
			for _, update := range intervalUpdates {
				updates = append(updates, trainingSheetPreviewUpdate{
					Update: update, Section: "intervals", Label: intervalLabelForRange(table, update.Range),
					RepairZeroClockHR: heartColumns[sheetRangeColumn(update.Range)],
				})
			}
		}
	}

	if cell := feedbackCellForPlanned(planned); cell != "" && strings.TrimSpace(activity.Feedback) != "" {
		updates = append(updates, trainingSheetPreviewUpdate{
			Update:  googleValueRangeUpdate{Range: sheetCellRange(planned.SheetTitle, cell), Values: [][]any{{strings.TrimSpace(activity.Feedback)}}},
			Section: "feedback", Label: "How did it feel/go?", ReplaceExisting: true,
		})
	} else if cell := feedbackCellForPlanned(planned); cell != "" {
		warnings = append(warnings, "Feedback is waiting for a saved reflection.")
	}
	return updates, warnings
}

func summaryUpdatesForActivity(planned PlannedActivity, activity Activity) []googleValueRangeUpdate {
	column := strings.TrimRight(planned.PlanCell, "0123456789")
	updates := make([]googleValueRangeUpdate, 0, 6)
	add := func(row string, value any) {
		updates = append(updates, googleValueRangeUpdate{Range: sheetCellRange(planned.SheetTitle, column+row), Values: [][]any{{value}}})
	}
	add("3", math.Round(activity.DistanceM/10)/100)
	duration := activity.MovingTimeS
	if duration <= 0 {
		duration = activity.ElapsedTimeS
	}
	if duration > 0 {
		add("4", "'"+sheetDurationText(duration))
	}
	if activity.AvgPaceSPKM != nil {
		add("5", "'"+sheetPaceText(*activity.AvgPaceSPKM))
	}
	if activity.AvgHeartRate != nil {
		add("6", math.Round(*activity.AvgHeartRate))
	}
	if activity.MaxHeartRate != nil {
		add("7", math.Round(*activity.MaxHeartRate))
	}
	if activity.RPE != nil {
		add("8", *activity.RPE)
	}
	return updates
}

func summaryLabelForRange(rangeName string) string {
	cell := rangeName[strings.LastIndex(rangeName, "!")+1:]
	row := strings.TrimLeft(cell, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	switch row {
	case "3":
		return "Distance"
	case "4":
		return "Duration"
	case "5":
		return "Avg Pace"
	case "6":
		return "Avg HR"
	case "7":
		return "HR MAX"
	case "8":
		return "RPE"
	default:
		return "Summary"
	}
}

func intervalLabelForRange(table *trainingSheetWorkoutTable, rangeName string) string {
	cell := rangeName[strings.LastIndex(rangeName, "!")+1:]
	rowText := strings.TrimLeft(cell, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	row, _ := strconv.Atoi(rowText)
	for _, item := range table.Rows {
		if item.Row == row {
			return item.Label
		}
	}
	return "Structured interval"
}

func trainingSheetPreviewRanges(updates []trainingSheetPreviewUpdate) []string {
	ranges := make([]string, len(updates))
	for index, item := range updates {
		ranges[index] = item.Update.Range
	}
	return ranges
}

type trainingSheetPreviewRegionSpec struct {
	rangeName string
	endRow    int
	endColumn int
}

type trainingSheetPreviewGrid struct {
	cells        map[string]trainingSheetPreviewGridCell
	merges       []googleGridRange
	columnWidths map[int]int
	columnHidden map[int]bool
	rowHeights   map[int]int
}

type trainingSheetPreviewGridCell struct {
	value string
	style *TrainingSheetPreviewCellStyle
}

type trainingSheetPreviewChangeInfo struct {
	current  string
	proposed string
	status   string
	section  string
	label    string
}

func trainingSheetPreviewRegion(planned PlannedActivity) trainingSheetPreviewRegionSpec {
	endRow := 40
	if values := stringMatrixFromAny(planned.Raw["values"]); len(values) > endRow {
		endRow = len(values)
	}
	if table := workoutTableFromPlanned(planned); table != nil {
		for _, row := range table.Rows {
			if row.Row+2 > endRow {
				endRow = row.Row + 2
			}
		}
	}
	if endRow > 250 {
		endRow = 250
	}
	return trainingSheetPreviewRegionSpec{
		rangeName: fmt.Sprintf("'%s'!A1:I%d", strings.ReplaceAll(planned.SheetTitle, "'", "''"), endRow),
		endRow:    endRow,
		endColumn: 9,
	}
}

func firstPreviewRangeValues(ranges [][][]string) [][]string {
	if len(ranges) == 0 {
		return nil
	}
	return ranges[0]
}

func trainingSheetPreviewGridFromGoogle(region trainingSheetPreviewRegionSpec, response googleSheetPreviewResponseSheet) trainingSheetPreviewGrid {
	grid := trainingSheetPreviewGrid{
		cells:        make(map[string]trainingSheetPreviewGridCell),
		merges:       append([]googleGridRange(nil), response.Merges...),
		columnWidths: make(map[int]int),
		columnHidden: make(map[int]bool),
		rowHeights:   make(map[int]int),
	}
	for _, data := range response.Data {
		startRow, startColumn := data.StartRow, data.StartColumn
		for rowOffset, row := range data.RowData {
			for columnOffset, cell := range row.Values {
				grid.cells[trainingSheetPreviewCellKey(startRow+rowOffset, startColumn+columnOffset)] = trainingSheetPreviewGridCell{
					value: cell.FormattedValue,
					style: previewCellStyle(cell.EffectiveFormat),
				}
			}
		}
		for offset, metadata := range data.RowMetadata {
			if metadata.PixelSize > 0 {
				grid.rowHeights[startRow+offset] = metadata.PixelSize
			}
		}
		for offset, metadata := range data.ColumnMetadata {
			column := startColumn + offset
			if metadata.PixelSize > 0 {
				grid.columnWidths[column] = metadata.PixelSize
			}
			if metadata.Hidden {
				grid.columnHidden[column] = true
			}
		}
	}
	return grid
}

func trainingSheetPreviewGridFromValues(region trainingSheetPreviewRegionSpec, values [][]string) trainingSheetPreviewGrid {
	grid := trainingSheetPreviewGrid{
		cells:        make(map[string]trainingSheetPreviewGridCell),
		columnWidths: make(map[int]int),
		columnHidden: make(map[int]bool),
		rowHeights:   make(map[int]int),
	}
	for rowIndex, row := range values {
		for columnIndex, value := range row {
			if value != "" {
				grid.cells[trainingSheetPreviewCellKey(rowIndex, columnIndex)] = trainingSheetPreviewGridCell{value: value}
			}
		}
	}
	return grid
}

func trainingSheetPreviewGridCurrentValues(grid trainingSheetPreviewGrid, rows, columns int) [][]string {
	values := make([][]string, rows)
	for row := 0; row < rows; row++ {
		values[row] = make([]string, columns)
		for column := 0; column < columns; column++ {
			values[row][column] = grid.cells[trainingSheetPreviewCellKey(row, column)].value
		}
	}
	return values
}

func trainingSheetPreviewExistingValues(grid trainingSheetPreviewGrid, updates []trainingSheetPreviewUpdate) [][][]string {
	values := make([][][]string, len(updates))
	for index, item := range updates {
		row, column, ok := previewCellCoordinates(item.Update.Range)
		if !ok {
			values[index] = [][]string{{}}
			continue
		}
		values[index] = [][]string{{grid.cells[trainingSheetPreviewCellKey(row, column)].value}}
	}
	return values
}

func trainingSheetPreviewGridResponse(grid trainingSheetPreviewGrid, region trainingSheetPreviewRegionSpec, planned PlannedActivity, changes []TrainingSheetPreviewChange, formattingAvailable bool) TrainingSheetPreviewGrid {
	values := trainingSheetPreviewGridCurrentValues(grid, region.endRow, region.endColumn)
	values = trimPreviewGridValues(values)
	day := -1
	if _, column, ok := previewCellCoordinates(planned.PlanCell); ok {
		day = column - 1
	}
	endRow := 8
	if day >= 0 && day <= 6 {
		for _, section := range trainingSheetSections(values) {
			if containsInt(section.Days, day) {
				if section.End > endRow {
					endRow = section.End
				}
				break
			}
		}
		if table := workoutTableForDay(values, day); table != nil {
			for _, row := range table.Rows {
				if row.Row > endRow {
					endRow = row.Row
				}
			}
		}
	}
	if endRow > region.endRow {
		endRow = region.endRow
	}
	changeByCell := make(map[string]trainingSheetPreviewChangeInfo, len(changes))
	for _, change := range changes {
		row, column, ok := previewCellCoordinates(change.Range)
		if !ok {
			continue
		}
		changeByCell[trainingSheetPreviewCellKey(row, column)] = trainingSheetPreviewChangeInfo{
			current: change.CurrentValue, proposed: change.ProposedValue, status: change.Status,
			section: change.Section, label: change.Label,
		}
	}
	columns := make([]TrainingSheetPreviewColumn, 0, region.endColumn)
	for column := 0; column < region.endColumn; column++ {
		columns = append(columns, TrainingSheetPreviewColumn{
			Index: column + 1, Label: spreadsheetColumn(column + 1), WidthPx: grid.columnWidths[column], Hidden: grid.columnHidden[column],
		})
	}
	rows := make([]TrainingSheetPreviewRow, 0, endRow)
	for row := 0; row < endRow; row++ {
		cells := make([]TrainingSheetPreviewCell, 0, region.endColumn)
		for column := 0; column < region.endColumn; column++ {
			rowSpan, columnSpan, merged := previewMergeSpan(grid.merges, row, column, endRow, region.endColumn)
			if merged && rowSpan == 0 && columnSpan == 0 {
				continue
			}
			raw := grid.cells[trainingSheetPreviewCellKey(row, column)]
			current := raw.value
			display := current
			status := "unchanged"
			section, label := "", ""
			proposed := ""
			if info, ok := changeByCell[trainingSheetPreviewCellKey(row, column)]; ok {
				display = info.proposed
				proposed = info.proposed
				status, section, label = info.status, info.section, info.label
			}
			cell := TrainingSheetPreviewCell{
				Ref: fmt.Sprintf("%s%d", spreadsheetColumn(column+1), row+1), CurrentValue: current,
				DisplayValue: display, ProposedValue: proposed, Status: status, Section: section, Label: label, Style: raw.style,
			}
			if rowSpan > 1 {
				cell.RowSpan = rowSpan
			}
			if columnSpan > 1 {
				cell.ColumnSpan = columnSpan
			}
			cells = append(cells, cell)
		}
		rows = append(rows, TrainingSheetPreviewRow{Index: row + 1, HeightPx: grid.rowHeights[row], Cells: cells})
	}
	return TrainingSheetPreviewGrid{
		StartRow: 1, EndRow: endRow, StartColumn: 1, EndColumn: region.endColumn,
		FormattingAvailable: formattingAvailable, Columns: columns, Rows: rows,
	}
}

func previewMergeSpan(merges []googleGridRange, row, column, endRow, endColumn int) (int, int, bool) {
	for _, merge := range merges {
		if row < merge.StartRowIndex || row >= merge.EndRowIndex || column < merge.StartColumnIndex || column >= merge.EndColumnIndex {
			continue
		}
		if row != merge.StartRowIndex || column != merge.StartColumnIndex {
			return 0, 0, true
		}
		rowSpan := merge.EndRowIndex - merge.StartRowIndex
		columnSpan := merge.EndColumnIndex - merge.StartColumnIndex
		if merge.EndRowIndex > endRow {
			rowSpan = endRow - merge.StartRowIndex
		}
		if merge.EndColumnIndex > endColumn {
			columnSpan = endColumn - merge.StartColumnIndex
		}
		return rowSpan, columnSpan, true
	}
	return 1, 1, false
}

func previewCellCoordinates(rangeName string) (int, int, bool) {
	cell := rangeName
	if separator := strings.LastIndex(cell, "!"); separator >= 0 {
		cell = cell[separator+1:]
	}
	cell = strings.ReplaceAll(strings.Trim(cell, " '"), "$", "")
	letters := 0
	for letters < len(cell) && cell[letters] >= 'A' && cell[letters] <= 'Z' {
		letters++
	}
	if letters == 0 || letters == len(cell) {
		return 0, 0, false
	}
	column := 0
	for _, value := range cell[:letters] {
		column = column*26 + int(value-'A'+1)
	}
	var row int
	if _, err := fmt.Sscanf(cell[letters:], "%d", &row); err != nil || row < 1 {
		return 0, 0, false
	}
	return row - 1, column - 1, true
}

func trimPreviewGridValues(values [][]string) [][]string {
	lastRow := 0
	for rowIndex, row := range values {
		for _, value := range row {
			if strings.TrimSpace(value) != "" {
				lastRow = rowIndex + 1
				break
			}
		}
	}
	if lastRow == 0 {
		return values[:0]
	}
	return values[:lastRow]
}

func trainingSheetPreviewCellKey(row, column int) string {
	return fmt.Sprintf("%d:%d", row, column)
}

func previewCellStyle(format googleCellFormat) *TrainingSheetPreviewCellStyle {
	style := &TrainingSheetPreviewCellStyle{
		BackgroundColor: previewGoogleColor(format.BackgroundColor, format.BackgroundColorStyle),
		TextColor:       previewGoogleColor(format.TextFormat.ForegroundColor, format.TextFormat.ForegroundColorStyle),
		Bold:            format.TextFormat.Bold, Italic: format.TextFormat.Italic,
		FontSize: int(math.Round(format.TextFormat.FontSize)), HorizontalAlignment: format.HorizontalAlignment,
		VerticalAlignment: format.VerticalAlignment, WrapStrategy: format.WrapStrategy,
	}
	if style.BackgroundColor == "" && style.TextColor == "" && !style.Bold && !style.Italic && style.FontSize == 0 && style.HorizontalAlignment == "" && style.VerticalAlignment == "" && style.WrapStrategy == "" {
		return nil
	}
	return style
}

func previewGoogleColor(color *googleColor, style *googleColorStyle) string {
	if color == nil && style != nil {
		color = style.RGBColor
	}
	if color == nil {
		return ""
	}
	red := int(math.Round(clampPreviewColor(color.Red) * 255))
	green := int(math.Round(clampPreviewColor(color.Green) * 255))
	blue := int(math.Round(clampPreviewColor(color.Blue) * 255))
	alpha := color.Alpha
	if alpha <= 0 {
		alpha = 1
	}
	if alpha >= 0.995 {
		return fmt.Sprintf("#%02x%02x%02x", red, green, blue)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %.2f)", red, green, blue, alpha)
}

func clampPreviewColor(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func trainingSheetPreviewCurrentValue(existing [][][]string, index int) string {
	if index >= len(existing) || len(existing[index]) == 0 || len(existing[index][0]) == 0 {
		return ""
	}
	return strings.TrimSpace(existing[index][0][0])
}

func previewValueText(values [][]any) string {
	if len(values) == 0 || len(values[0]) == 0 || values[0][0] == nil {
		return ""
	}
	value := fmt.Sprint(values[0][0])
	return strings.TrimPrefix(value, "'")
}

func trainingSheetPreviewStatus(item trainingSheetPreviewUpdate, current, proposed string) string {
	if current == "" {
		return "write"
	}
	if current == proposed {
		return "unchanged"
	}
	if item.ReplaceExisting || (item.RepairZeroClockHR && (current == "0:00" || current == "0:00:00")) {
		return "write"
	}
	return "conflict"
}

func trainingSheetPreviewFingerprint(plannedID, activityID string, updates []trainingSheetPreviewUpdate, existing [][][]string) string {
	values := make([]map[string]string, len(updates))
	for index, item := range updates {
		current := ""
		if index < len(existing) {
			current = trainingSheetPreviewCurrentValue(existing, index)
		}
		values[index] = map[string]string{"range": item.Update.Range, "current": current, "proposed": previewValueText(item.Update.Values)}
	}
	payload, _ := json.Marshal(map[string]any{"plannedActivityId": plannedID, "activityId": activityID, "values": values})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}
