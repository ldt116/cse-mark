package coursequery

import (
	"thuanle/cse-mark/internal/domain/course"
	"time"
)

type ActiveCourseService struct {
	CourseRepo course.Repository
	Rule       *course.Rules
}

func NewActiveCourseService(r course.Repository, rule *course.Rules) *ActiveCourseService {
	return &ActiveCourseService{
		CourseRepo: r,
		Rule:       rule,
	}
}

func (s *ActiveCourseService) ListActiveCourses() ([]course.Model, error) {
	threshold := time.Now().Add(-s.Rule.CourseActiveAge)

	actives, err := s.CourseRepo.FindCoursesUpdatedAfter(threshold)
	if err != nil {
		return nil, err
	}

	return actives, nil
}
