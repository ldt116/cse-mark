package markimport

import (
	"errors"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/downloader"
	"thuanle/cse-mark/internal/domain/mark"

	"github.com/rs/zerolog/log"
)

type Service struct {
	courseRepo course.Repository
	markRepo   mark.Repository

	downloader downloader.Repository
}

func NewService(downloader downloader.Repository,
	courseRepo course.Repository, markRepo mark.Repository) *Service {
	return &Service{
		courseRepo: courseRepo,
		markRepo:   markRepo,

		downloader: downloader,
	}
}

func (s *Service) CleanRawCsvRecords(records [][]string) ([]map[string]string, error) {
	if len(records) < 2 {
		return nil, errors.New("invalid csv structure")
	}

	flags := records[0]
	headers := records[1]
	data := records[2:]

	var cleaned []map[string]string

	if len(flags) == 0 {
		return cleaned, nil
	}

	for _, row := range data {
		item := make(map[string]string)
		for flagIndex, flag := range flags {
			if len(flag) > 0 {
				if flag == "id" {
					item["_id"] = row[flagIndex]
					item[flag] = row[flagIndex]
				} else {
					item[headers[flagIndex]] = row[flagIndex]
				}
			}
		}

		cleaned = append(cleaned, item)
	}

	return cleaned, nil

}

func (s *Service) FetchMarkLinkIntoCourse(courseId string, link string) (int, error) {
	log.Info().
		Str("courseId", courseId).
		Str("link", link).
		Msg("Fetching new marks")

	records, err := s.downloader.DownloadCSV(link)
	if err != nil {
		return 0, err
	}

	log.Debug().
		Str("courseId", courseId).
		Strs("Flags", records[0]).
		Strs("Headers", records[1]).
		Msg("Record fetched")

	cleanedRecords, err := s.CleanRawCsvRecords(records)
	if err != nil {
		return 0, err
	}

	log.Debug().Interface("cleanData", cleanedRecords).Msg("Cleaned data")

	log.Debug().Msg("Clearing old course marks ")
	err = s.markRepo.RemoveMarksByCourseId(courseId)
	if err != nil {
		return 0, err
	}

	log.Debug().Msg("Storing new course marks")
	err = s.markRepo.AddCourseMarks(courseId, cleanedRecords)
	if err != nil {
		return 0, err
	}

	recordsCount := len(cleanedRecords)
	err = s.courseRepo.UpdateCourseRecordCount(courseId, recordsCount)

	log.Info().
		Str("course", courseId).
		Str("link", link).
		Int("records", len(cleanedRecords)).
		Msg("Fetched new marks")

	return recordsCount, nil
}
