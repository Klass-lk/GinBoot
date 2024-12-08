package ginboot

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

type AuthContext struct {
	UserID    string
	UserEmail string
	Roles     []string
	Claims    map[string]interface{}
}

type Context struct {
	*gin.Context
	authContext *AuthContext
}

func NewContext(c *gin.Context) *Context {
	return &Context{
		Context: c,
	}
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
