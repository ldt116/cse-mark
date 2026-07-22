package courseadmin

import "errors"

var (
	ErrInvalidCourseId = errors.New("invalid course id")
	ErrInvalidLink     = errors.New("invalid csv link")
	ErrNotFound        = errors.New("course not found")
)
