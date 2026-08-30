package handlers

import (
	"github.com/gin-gonic/gin"

	"thuanle/cse-mark/internal/delivery/api/errors"
	"thuanle/cse-mark/internal/usecases/marksquery"
)

// StudentMarks serves GET /marks: one student's marks across courses,
// authenticated by the student-app JWT (see the Jwt middleware).
type StudentMarks struct {
	query *marksquery.Service
}

func NewStudentMarksHandler(query *marksquery.Service) *StudentMarks {
	return &StudentMarks{
		query: query,
	}
}

// GetAll returns the authenticated student's marks. The student id comes
// from the "sub" context value set by the JWT middleware — never from a
// query parameter, so a student can only ever read their own marks. The
// optional course_id query parameter narrows the result to one course.
func (h *StudentMarks) GetAll(c *gin.Context) {
	sub := c.GetString("sub")
	out, err := h.query.Query(sub, c.Query("course_id"))
	if err != nil {
		errors.InternalError(c)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, out)
}
