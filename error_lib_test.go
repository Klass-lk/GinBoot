package ginboot

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestApiError_NewApiError(t *testing.T) {
	err := NewApiError(404, "User %s not found")
	assert.Equal(t, "404", err.ErrorCode)
	assert.Equal(t, "User %s not found", err.Message)
	assert.Equal(t, "404: User %s not found", err.Error())
}

func TestApiError_NewFormatting(t *testing.T) {
	baseErr := NewApiError(400, "Invalid parameter: %s in field %s")
	formattedErr := baseErr.New("email", "user_email")

	assert.Equal(t, "400", formattedErr.ErrorCode)
	assert.Equal(t, "Invalid parameter: email in field user_email", formattedErr.Message)
	assert.Equal(t, "400: Invalid parameter: email in field user_email", formattedErr.Error())
}

func TestSendError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
		expectedMsg    string
	}{
		{
			name:           "ApiError with status 404",
			err:            NewApiError(404, "Resource not found"),
			expectedStatus: http.StatusNotFound,
			expectedCode:   "404",
			expectedMsg:    "Resource not found",
		},
		{
			name:           "ApiError with non-status numeric code defaults to 400",
			err:            ApiError{ErrorCode: "1001", Message: "Custom app error"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "1001",
			expectedMsg:    "Custom app error",
		},
		{
			name:           "Generic std error",
			err:            errors.New("db connection failure"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   "Internal Server Error",
			expectedMsg:    "db connection failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			SendError(c, tt.err)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedCode)
			assert.Contains(t, w.Body.String(), tt.expectedMsg)
		})
	}
}
