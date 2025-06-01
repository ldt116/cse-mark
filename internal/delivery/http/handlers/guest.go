package handlers

import (
	"net/http"
	"thuanle/cse-mark/internal/configs"
	"thuanle/cse-mark/internal/domain/mark"
	"thuanle/cse-mark/internal/domain/user"

	"github.com/gin-gonic/gin"
)

type GuestHandler struct {
	markRepo mark.Repository
	config   *configs.Config
}

func NewGuestHandler(markRepo mark.Repository, config *configs.Config) *GuestHandler {
	return &GuestHandler{markRepo, config}
}

func (h *GuestHandler) LookupMark(c *gin.Context) {
	// key := c.Query("key")
	// if key == "" || key != h.config.ApiSecretKey {
	// 	c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	// 	return
	// }

	courseId := c.Query("subject")
	studentId := c.Query("id")

	if !user.IsValidStudentId(studentId) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid student id"})
		return
	}

	markJson, err := h.markRepo.GetMark(courseId, studentId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	c.Data(http.StatusOK, "application/json", []byte(markJson))
}
