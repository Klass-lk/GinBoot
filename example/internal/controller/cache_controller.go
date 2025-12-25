package controller

import (
	"context"

	"github.com/klass-lk/ginboot"
)

type CacheController struct {
	cacheService ginboot.CacheService
}

func NewCacheController(cacheService ginboot.CacheService) *CacheController {
	return &CacheController{
		cacheService: cacheService,
	}
}

func (c *CacheController) Register(group *ginboot.ControllerGroup) {
	group.POST("/invalidate", c.Invalidate)
}

// Invalidate handles manual cache invalidation requests
// Query param: tag (required)
func (c *CacheController) Invalidate(ctx *ginboot.Context) (ginboot.EmptyResponse, error) {
	tag := ctx.Query("tag")
	if tag == "" {
		return ginboot.EmptyResponse{}, ginboot.ApiError{ErrorCode: "BAD_REQUEST", Message: "Tag is required"}
	}

	err := c.cacheService.Invalidate(context.Background(), tag)
	if err != nil {
		return ginboot.EmptyResponse{}, err
	}

	return ginboot.EmptyResponse{}, nil
}
