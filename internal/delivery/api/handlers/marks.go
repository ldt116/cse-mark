package handlers

import (
	"github.com/gin-gonic/gin"
	"thuanle/cse-mark/internal/delivery/api/errors"
	"thuanle/cse-mark/internal/domain/mark"
)

type Marks struct {
	markRepo mark.Repository
}

func NewMarksHandler(markRepo mark.Repository) *Marks {
	return &Marks{
		markRepo: markRepo,
	}
}

// GetMark handles the request to get marks
func (h *Marks) GetMark(c *gin.Context) {
	courseId := c.Query("course")
	studentId := c.Query("student")
	if courseId == "" || studentId == "" {
		errors.BadRequest(c)
		return
	}

	marks, err := h.markRepo.GetMark(courseId, studentId)
	if err != nil {
		errors.BadRequest(c)
		return
	}

	c.String(200, marks)
}
