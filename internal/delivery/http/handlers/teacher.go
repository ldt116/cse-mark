package handlers

import (
	"net/http"
	"thuanle/cse-mark/internal/usecases/markimport"

	"github.com/gin-gonic/gin"
)

type TeacherHandler struct {
	markImportService *markimport.Service
}

func NewTeacherHandler(markImportService *markimport.Service) *TeacherHandler {
	return &TeacherHandler{
		markImportService: markImportService,
	}
}

func (h *TeacherHandler) LoadCourseLink(c *gin.Context) {
	var req struct {
		CourseId string `json:"course_id"`
		Link     string `json:"link"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	count, err := h.markImportService.FetchMarkLinkIntoCourse(req.CourseId, req.Link)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"course_id": req.CourseId,
		"link":      req.Link,
		"records":   count,
		"status":    "ok",
	})

}
