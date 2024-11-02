package main

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot/types"
	"net/http"
	"strconv"
	"strings"
)

func GetAuthContext(c *gin.Context) (types.AuthContext, error) {
	userId, exists := c.Get("user_id")
	if !exists {
		c.AbortWithStatus(http.StatusUnauthorized)
		return types.AuthContext{}, errors.New("operation not permitted")
	}
	role, exists := c.Get("role")
	if !exists {
		c.AbortWithStatus(http.StatusUnauthorized)
		return types.AuthContext{}, errors.New("operation not permitted")
	}
	return types.AuthContext{
		UserId: userId.(string),
		Role:   role.(string),
	}, nil
}

func BuildAuthRequestContext[T interface{}](c *gin.Context) (T, types.AuthContext, error) {
	request, err := BuildRequest[T](c)
	if err != nil {
		return request, types.AuthContext{}, err
	}
	authContext, err := GetAuthContext(c)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return request, types.AuthContext{}, err
	}
	return request, authContext, nil
}

func BuildPageRequest(c *gin.Context) types.PageRequest {
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
	var sort types.SortField
	if len(sortSplit) > 1 {
		direction := 1
		if sortSplit[1] == "desc" {
			direction = -1
		}
		sort = types.SortField{
			Field:     sortSplit[0],
			Direction: direction,
		}
	} else {
		sort = types.SortField{
			Field:     sortSplit[0],
			Direction: 1,
		}
	}

	return types.PageRequest{Page: int(page), Size: int(size), Sort: sort}
}

func BuildRequest[T interface{}](c *gin.Context) (T, error) {
	var request T
	if c.ShouldBindJSON(&request) != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return request, errors.New("bad request")
	}
	return request, nil
}
