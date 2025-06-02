package iam

import (
	"errors"
	"thuanle/cse-mark/internal/domain/course"
	"thuanle/cse-mark/internal/domain/user"
)

type AuthzService struct {
	courseRepo course.Repository
	userRepo   user.Repository
}

func NewAuthzService(courseRepo course.Repository, userRepo user.Repository) *AuthzService {
	return &AuthzService{
		courseRepo: courseRepo,
		userRepo:   userRepo,
	}
}

// CanEditCourse checks if the user can edit the course.
func (s *AuthzService) CanEditCourse(username string, teleId int64, courseId string) (bool, error) {
	courseModel, err := s.courseRepo.FindCourseById(courseId)

	if err != nil {
		if errors.Is(err, course.ErrNotFound) {
			// course does not exist → grant
			return true, nil
		}
		return false, err
	}

	if username != "" && courseModel.ByTeleUser == username {
		return true, nil
	}
	if teleId != 0 && courseModel.ByTeleId == teleId {
		return true, nil
	}
	return false, nil
}

func (s *AuthzService) IsTeacher(username string) (bool, error) {
	userModel, err := s.userRepo.FindUserById(username)
	if err != nil {
		return false, err
	}

	return userModel.IsTeacher, nil
}
