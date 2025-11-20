package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Health struct{}

func NewHealthHandler() *Health {
	return &Health{}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// Check performs health check - simple liveness probe
func (h *Health) Check(c *gin.Context) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, response)
}