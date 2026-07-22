package app

import "testing"

func TestTrainingSheetPreviewStatusClassifiesWritesAndConflicts(t *testing.T) {
	base := trainingSheetPreviewUpdate{Update: googleValueRangeUpdate{Range: "'26-07'!C19", Values: [][]any{{"'163"}}}}
	if got := trainingSheetPreviewStatus(base, "", "163"); got != "write" {
		t.Fatalf("blank status = %q, want write", got)
	}
	if got := trainingSheetPreviewStatus(base, "163", "163"); got != "unchanged" {
		t.Fatalf("same status = %q, want unchanged", got)
	}
	if got := trainingSheetPreviewStatus(base, "160", "163"); got != "conflict" {
		t.Fatalf("conflict status = %q, want conflict", got)
	}
	base.ReplaceExisting = true
	if got := trainingSheetPreviewStatus(base, "160", "163"); got != "write" {
		t.Fatalf("replacement status = %q, want write", got)
	}
	base.ReplaceExisting = false
	base.RepairZeroClockHR = true
	if got := trainingSheetPreviewStatus(base, "0:00", "163"); got != "write" {
		t.Fatalf("zero-clock repair status = %q, want write", got)
	}
}

func TestTrainingSheetPreviewFingerprintIncludesCurrentAndProposedValues(t *testing.T) {
	updates := []trainingSheetPreviewUpdate{{
		Update: googleValueRangeUpdate{Range: "'26-07'!C19", Values: [][]any{{"'163"}}},
	}}
	first := trainingSheetPreviewFingerprint("planned", "activity", updates, [][][]string{{{"160"}}})
	second := trainingSheetPreviewFingerprint("planned", "activity", updates, [][][]string{{{"161"}}})
	if first == second {
		t.Fatal("fingerprint did not change when the current sheet value changed")
	}
}

func TestApplyTrainingSheetPreviewDraft(t *testing.T) {
	feedback := "  felt controlled  "
	activity := Activity{Feedback: "old"}
	applyTrainingSheetPreviewDraft(&activity, trainingSheetPreviewRequest{Feedback: &feedback, RPESet: true, RPE: intPointer(8)})
	if activity.Feedback != "felt controlled" {
		t.Fatalf("feedback = %q, want trimmed draft", activity.Feedback)
	}
	if activity.RPE == nil || *activity.RPE != 8 {
		t.Fatalf("RPE = %#v, want 8", activity.RPE)
	}
}

func TestTrainingSheetPreviewGridResponseFocusesSelectedWorkoutSection(t *testing.T) {
	values := make([][]string, 13)
	for index := range values {
		values[index] = make([]string, 9)
	}
	values[0][1] = "Mon"
	values[0][2] = "Tues"
	values[0][3] = "Wed"
	values[1][3] = "5x7mins"
	values[2][0] = "Distance"
	values[3][0] = "Time"
	values[4][0] = "Avg Pace"
	values[5][0] = "Avg HR"
	values[6][0] = "HR MAX"
	values[7][0] = "RPE"
	values[9][0] = "Wednesday:15mins easy warm up//5x7mins"
	values[10][1] = "Avg Pace"
	values[10][2] = "Avg HR"
	values[11][0] = "7min rep 1"
	values[11][1] = "4:00"
	values[12][0] = "How did it feel/go?"

	planned := PlannedActivity{PlanCell: "D2", SheetTitle: "26-07", Raw: map[string]any{"values": values}}
	grid := trainingSheetPreviewGridFromValues(trainingSheetPreviewRegion(planned), values)
	changes := []TrainingSheetPreviewChange{
		{Range: "'26-07'!D3", Section: "summary", Label: "Distance", CurrentValue: "", ProposedValue: "12.3", Status: "write"},
		{Range: "'26-07'!B12", Section: "intervals", Label: "7min rep 1", CurrentValue: "4:00", ProposedValue: "3:35", Status: "conflict"},
	}

	preview := trainingSheetPreviewGridResponse(grid, trainingSheetPreviewRegion(planned), planned, changes, false)
	if preview.EndRow != 13 {
		t.Fatalf("focused end row = %d, want 13", preview.EndRow)
	}
	if len(preview.Rows) != 13 || len(preview.Columns) != 9 {
		t.Fatalf("grid dimensions = %dx%d, want 13x9", len(preview.Rows), len(preview.Columns))
	}
	if preview.FormattingAvailable {
		t.Fatal("formatting availability = true, want false")
	}

	findCell := func(ref string) TrainingSheetPreviewCell {
		for _, row := range preview.Rows {
			for _, cell := range row.Cells {
				if cell.Ref == ref {
					return cell
				}
			}
		}
		t.Fatalf("cell %s was not found", ref)
		return TrainingSheetPreviewCell{}
	}
	writeCell := findCell("D3")
	if writeCell.DisplayValue != "12.3" || writeCell.Status != "write" {
		t.Fatalf("D3 = %#v, want proposed write cell", writeCell)
	}
	conflictCell := findCell("B12")
	if conflictCell.DisplayValue != "3:35" || conflictCell.CurrentValue != "4:00" || conflictCell.Status != "conflict" {
		t.Fatalf("B12 = %#v, want proposed conflict cell", conflictCell)
	}
}

func TestPreviewCellStyleConvertsGoogleColors(t *testing.T) {
	style := previewCellStyle(googleCellFormat{
		BackgroundColor:     &googleColor{Red: 1, Green: 0.5, Blue: 0, Alpha: 1},
		TextFormat:          googleTextFormat{ForegroundColor: &googleColor{Blue: 1, Alpha: 1}, Bold: true, FontSize: 11},
		HorizontalAlignment: "CENTER",
		WrapStrategy:        "WRAP",
	})
	if style == nil {
		t.Fatal("style = nil")
	}
	if style.BackgroundColor != "#ff8000" || style.TextColor != "#0000ff" || !style.Bold || style.FontSize != 11 || style.HorizontalAlignment != "CENTER" || style.WrapStrategy != "WRAP" {
		t.Fatalf("style = %#v", style)
	}
}

func TestPreviewCellCoordinatesSupportsQuotedSheetRanges(t *testing.T) {
	row, column, ok := previewCellCoordinates("'26-07'!$C$19")
	if !ok || row != 18 || column != 2 {
		t.Fatalf("coordinates = (%d, %d, %t), want (18, 2, true)", row, column, ok)
	}
}

func intPointer(value int) *int {
	return &value
}
