package rostersync

import (
	"strings"

	"thuanle/cse-mark/internal/domain/student"
)

// utf8BOM is the byte-order mark Excel prepends to CSV exports; Go's
// encoding/csv leaves it on the first cell, where strings.TrimSpace does not
// remove it. Defined via escape so the source itself stays BOM-free.
const utf8BOM = "\ufeff"

// parseRoster converts raw CSV records into roster students.
//
// The roster CSV has exactly three columns in fixed order: MSSV, Name, Email
// (see docs/v2/SRS-v2.md §9.2). Rows that are not valid student data are
// skipped so a header row, blank lines, or malformed rows never pollute the
// students collection:
//   - rows with fewer than three fields (index safety, avoids a panic),
//   - rows whose MSSV field is empty,
//   - a header row whose MSSV field is literally "MSSV" (case-insensitive).
//
// Leading/trailing whitespace is trimmed from every field, and a UTF-8 BOM (as
// emitted by Excel CSV exports) is stripped so the header row is still skipped.
// parseRoster never returns an error: structural CSV failures surface earlier at
// download time; skipped rows are counted by the caller for logging.
func parseRoster(records [][]string) []student.Model {
	models := make([]student.Model, 0, len(records))
	for _, row := range records {
		if len(row) < 3 {
			continue
		}
		mssv := strings.TrimSpace(row[0])
		mssv = strings.TrimPrefix(mssv, utf8BOM)
		if mssv == "" || strings.EqualFold(mssv, "MSSV") {
			continue
		}
		models = append(models, student.Model{
			MSSV:  mssv,
			Name:  strings.TrimSpace(row[1]),
			Email: strings.TrimSpace(row[2]),
		})
	}
	return models
}
