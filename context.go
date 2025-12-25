package ginboot

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type AuthContext struct {
	UserID    string
	UserEmail string
	Roles     []string
	Claims    map[string]interface{}
}

type Context struct {
	*gin.Context
	fileService FileService
}

func NewContext(c *gin.Context, fileService FileService) *Context {
	return &Context{
		Context:     c,
		fileService: fileService,
	}
}

func (c *Context) GetFileService() FileService {
	return c.fileService
}

// GetAuthContext returns the current auth context
func (c *Context) GetAuthContext() (AuthContext, error) {
	userId, exists := c.Get("user_id")
	if !exists {
		c.AbortWithStatus(http.StatusUnauthorized)
		return AuthContext{}, errors.New("operation not permitted")
	}
	role, exists := c.Get("role")
	if !exists {
		c.AbortWithStatus(http.StatusUnauthorized)
		return AuthContext{}, errors.New("operation not permitted")
	}
	return AuthContext{
		UserID: userId.(string),
		Roles:  []string{role.(string)},
	}, nil
}

func (c *Context) GetRequest(request interface{}) error {
	if err := c.ShouldBind(request); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return errors.New("bad request: " + err.Error())
	}
	return nil
}

func (c *Context) GetPageRequest() PageRequest {
	pageString := c.DefaultQuery("page", "1")
	sizeString := c.DefaultQuery("size", "10")
	sortString := c.DefaultQuery("sort", "_id,asc")
	page, err := strconv.ParseInt(pageString, 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
	}
	size, err := strconv.ParseInt(sizeString, 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
	}
	sortSplit := strings.Split(sortString, ",")
	var sort SortField
	if len(sortSplit) > 1 {
		direction := 1
		if sortSplit[1] == "desc" {
			direction = -1
		}
		sort = SortField{
			Field:     sortSplit[0],
			Direction: direction,
		}
	} else {
		sort = SortField{
			Field:     sortSplit[0],
			Direction: 1,
		}
	}

	return PageRequest{Page: int(page), Size: int(size), Sort: sort}
}

func (c *Context) SendError(err error) {
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
