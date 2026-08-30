// Package marksquery returns one student's marks across every course
// (grade-share feature #44). The student's identity has already been
// verified by the JWT assertion layer; this usecase only reads marks.
package marksquery

import (
	"encoding/json"
	"errors"
	"time"

	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/mark"
)

// epochZero mirrors the bot's /mark behaviour: every course is listed,
// so inactive courses still expose their frozen marks (Ruling 4, #43).
var epochZero = time.Unix(0, 0)

// CourseMarks is the marks JSON of one course, passed through verbatim.
type CourseMarks struct {
	CourseId string          `json:"courseId"`
	Marks    json.RawMessage `json:"marks"`
}

type Service struct {
	courseRepo course.Repository
	markRepo   mark.Repository
	rules      *course.Rules
}

func NewService(courseRepo course.Repository, markRepo mark.Repository, rules *course.Rules) *Service {
	return &Service{
		courseRepo: courseRepo,
		markRepo:   markRepo,
		rules:      rules,
	}
}

// Query returns the marks of studentId. A non-empty courseId restricts the
// result to that course; an empty courseId covers every course, skipping
// courses the student has no mark document in. An empty result is a
// non-nil empty slice, not nil.
func (s *Service) Query(studentId, courseId string) ([]CourseMarks, error) {
	if courseId != "" {
		// A malformed courseId (same rule as the tele bot) is not distinguishable
		// from "no data" (Ruling 5): answer 200 [] without touching Mongo, whose
		// db.Collection(courseId) would otherwise fail on an invalid name and
		// break the always-200 contract with a 500.
		if !s.rules.IsValidCourseId(courseId) {
			return []CourseMarks{}, nil
		}
		m, err := s.markRepo.GetMark(courseId, studentId)
		if err != nil {
			if errors.Is(err, mark.ErrNotFound) {
				return []CourseMarks{}, nil
			}
			return nil, err
		}
		return []CourseMarks{{CourseId: courseId, Marks: json.RawMessage(m)}}, nil
	}

	courses, err := s.courseRepo.FindCoursesUpdatedAfter(epochZero)
	if err != nil {
		return nil, err
	}

	out := make([]CourseMarks, 0, len(courses))
	for _, c := range courses {
		m, err := s.markRepo.GetMark(c.Id, studentId)
		if err != nil {
			if errors.Is(err, mark.ErrNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, CourseMarks{CourseId: c.Id, Marks: json.RawMessage(m)})
	}
	return out, nil
}
