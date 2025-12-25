package ginboot

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ApiError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func (e ApiError) New(messages ...string) ApiError {
	args := make([]any, len(messages))
	for i, msg := range messages {
		args[i] = msg
	}

	message := fmt.Sprintf(e.Message, args...)
	return ApiError{
		ErrorCode: e.ErrorCode,
		Message:   message,
	}
}

func (e ApiError) Error() string {
	return fmt.Sprintf("%s: %s", e.ErrorCode, e.Message)
}

type ErrorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func SendError(c *gin.Context, err error) {
	var customErr ApiError
	if errors.As(err, &customErr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error_code": customErr.ErrorCode,
			"message":    customErr.Message,
		})
		return
	}
	// Handle other types of errors here
	c.JSON(http.StatusInternalServerError, gin.H{
		"error_code": "Internal Server Error",
		"message":    "An unknown error occurred",
	})
}
