package rostersync

import (
	"reflect"
	"testing"

	"thuanle/cse-mark/internal/domain/student"
)

func TestParseRoster_ValidRows(t *testing.T) {
	got := parseRoster([][]string{
		{"2013307", "Nguyen Van A", "a@hcmut.edu.vn"},
		{"2013308", "Nguyen Van B", "b@hcmut.edu.vn"},
	})
	want := []student.Model{
		{MSSV: "2013307", Name: "Nguyen Van A", Email: "a@hcmut.edu.vn"},
		{MSSV: "2013308", Name: "Nguyen Van B", Email: "b@hcmut.edu.vn"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseRoster_SkipsHeaderAndInvalid(t *testing.T) {
	in := [][]string{
		{"\ufeffMSSV", "Name", "Email"},                     // header — skipped (EqualFold)
		{"mssv", "name", "email"},                     // header lowercase — skipped
		{"", "No ID", "x@hcmut.edu.vn"},               // empty MSSV — skipped
		{"2013307", "Only Two Cols"},                  // < 3 fields — skipped
		{"2013309", "  Trim Me  ", " c@hcmut.edu.vn "}, // whitespace trimmed
	}
	got := parseRoster(in)
	want := []student.Model{
		{MSSV: "2013309", Name: "Trim Me", Email: "c@hcmut.edu.vn"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseRoster_TakesFirstThreeWhenExtraColumns(t *testing.T) {
	got := parseRoster([][]string{
		{"2013310", "Extra", "d@hcmut.edu.vn", "note", "more"},
	})
	want := []student.Model{{MSSV: "2013310", Name: "Extra", Email: "d@hcmut.edu.vn"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseRoster_Empty(t *testing.T) {
	if got := parseRoster(nil); len(got) != 0 {
		t.Errorf("want empty, got %+v", got)
	}
}

// Excel CSV exports prepend a UTF-8 BOM to the first cell. Without stripping
// it, the header row's first cell is "\ufeffMSSV" and the header is not skipped
// (it would be upserted as a bogus student).
func TestParseRoster_StripsUTF8BOMOnHeader(t *testing.T) {
	got := parseRoster([][]string{
		{"\ufeffMSSV", "Name", "Email"},          // BOM-prefixed header — skipped
		{"\ufeff2013307", "BOM ID", "a@hcmut.edu.vn"}, // BOM-prefixed MSSV — stripped
	})
	want := []student.Model{
		{MSSV: "2013307", Name: "BOM ID", Email: "a@hcmut.edu.vn"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BOM handling: got %+v, want %+v", got, want)
	}
}

