package controller

import (
	"strconv"
	"strings"

	"github.com/klass-lk/ginboot/example/internal/middleware"

	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/model"
	"github.com/klass-lk/ginboot/example/internal/service"
)

type PostController struct {
	postService *service.PostService
}

func NewPostController(postService *service.PostService) *PostController {
	return &PostController{
		postService: postService,
	}
}

func (c *PostController) Register(group *ginboot.ControllerGroup) {
	group.GET("", c.GetPosts)
	group.GET("/:id", c.GetPost)
	group.GET("/author/:author", c.GetPostsByAuthor)
	group.GET("/tags/:tags", c.GetPostsByTags)

	protected := group.Group("", middleware.AuthMiddleware())
	{
		protected.POST("", c.CreatePost)
		protected.PUT("/:id", c.UpdatePost)
		protected.DELETE("/:id", c.DeletePost)
	}
}

func (c *PostController) CreatePost(request model.Post) (model.Post, error) {
	return c.postService.CreatePost(request)
}

func (c *PostController) GetPost(ctx *ginboot.Context) (model.Post, error) {
	id := ctx.Param("id")
	return c.postService.GetPostById(id)
}

func (c *PostController) UpdatePost(ctx *ginboot.Context, post model.Post) (model.Post, error) {
	id := ctx.Param("id")
	return post, c.postService.UpdatePost(id, post)
}

func (c *PostController) DeletePost(ctx *ginboot.Context) (ginboot.EmptyResponse, error) {
	id := ctx.Param("id")
	err := c.postService.DeletePost(id)
	if err != nil {
		return ginboot.EmptyResponse{}, err
	}
	return ginboot.EmptyResponse{}, nil
}

func (c *PostController) GetPosts(ctx *ginboot.Context) (ginboot.PageResponse[model.Post], error) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))
	sortField := ctx.DefaultQuery("sort", "created_at")
	sortDir, _ := strconv.Atoi(ctx.DefaultQuery("direction", "-1"))

	return c.postService.GetPosts(page, size, ginboot.SortField{
		Field:     sortField,
		Direction: sortDir,
	})
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
