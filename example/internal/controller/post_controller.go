package controller

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/model"
	"github.com/klass-lk/ginboot/example/internal/service"
)

type PostController struct {
	postService     *service.PostService
	cacheService    ginboot.CacheService
	cacheMiddleware gin.HandlerFunc
}

func NewPostController(postService *service.PostService, cacheService ginboot.CacheService, cacheMiddleware gin.HandlerFunc) *PostController {
	return &PostController{
		postService:     postService,
		cacheService:    cacheService,
		cacheMiddleware: cacheMiddleware,
	}
}

func (c *PostController) Register(group *ginboot.ControllerGroup) {
	// Apply cache middleware to list endpoints
	// We can generate tags in middleware using the TagGenerator,
	// or we can rely on Default Key Generation and manual invalidation.
	// For this example, let's assume middleware generates keys/tags.
	group.GET("", c.GetPosts, c.cacheMiddleware)
	group.GET("/:id", c.GetPost, c.cacheMiddleware)
	group.GET("/author/:author", c.GetPostsByAuthor, c.cacheMiddleware)
	group.GET("/tags/:tags", c.GetPostsByTags, c.cacheMiddleware)

	protected := group.Group("")
	{
		protected.POST("", c.CreatePost)
		protected.PUT("/:id", c.UpdatePost)
		protected.DELETE("/:id", c.DeletePost)
	}
}

func (c *PostController) CreatePost(request model.Post) (model.Post, error) {
	post, err := c.postService.CreatePost(request)
	if err == nil {
		// Invalidate "posts" tag on creation
		err = c.cacheService.Invalidate(context.Background(), "posts")
		if err != nil {
			return model.Post{}, err
		}
	}
	return post, err
}

func (c *PostController) GetPost(ctx *ginboot.Context) (model.Post, error) {
	id := ctx.Param("id")
	return c.postService.GetPostById(id)
}

func (c *PostController) UpdatePost(ctx *ginboot.Context, post model.Post) (model.Post, error) {
	id := ctx.Param("id")
	err := c.postService.UpdatePost(id, post)
	if err == nil {
		// Invalidate general list and specific item if tagged
		// For simplicity, invalidate global "posts" tag
		_ = c.cacheService.Invalidate(context.Background(), "posts")
	}
	return post, err
}

func (c *PostController) DeletePost(ctx *ginboot.Context) (ginboot.EmptyResponse, error) {
	id := ctx.Param("id")
	err := c.postService.DeletePost(id)
	if err != nil {
		return ginboot.EmptyResponse{}, err
	}
	_ = c.cacheService.Invalidate(context.Background(), "posts")
	return ginboot.EmptyResponse{}, nil
}

func (c *PostController) GetPosts(ctx *ginboot.Context) (ginboot.PageResponse[model.Post], error) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))
	sortField := ctx.DefaultQuery("sort", "created_at")
	sortDir, _ := strconv.Atoi(ctx.DefaultQuery("direction", "-1"))

	res, err := c.postService.GetPosts(page, size, ginboot.SortField{
		Field:     sortField,
		Direction: sortDir,
	})

	if err != nil {
		return ginboot.PageResponse[model.Post]{}, err
	}
	return res, nil
}

func (c *PostController) GetPostsByAuthor(ctx *ginboot.Context) (ginboot.PageResponse[model.Post], error) {
	author := ctx.Param("author")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))

	return c.postService.GetPostsByAuthor(author, page, size)
}

func (c *PostController) GetPostsByTags(ctx *ginboot.Context) (ginboot.PageResponse[model.Post], error) {
	tagsStr := ctx.Query("tags")
	tags := strings.Split(tagsStr, ",")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))

	return c.postService.GetPostsByTags(tags, page, size)
}
