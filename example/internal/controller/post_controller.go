package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/model"
	"github.com/klass-lk/ginboot/example/internal/service"
)

type PostController struct {
	postService *service.PostService
}

func (c *PostController) Routes() []ginboot.Route {
	return []ginboot.Route{
		{
			Method:  http.MethodPost,
			Path:    "",
			Handler: c.CreatePost,
		},
		{
			Method:  http.MethodGet,
			Path:    "/:id",
			Handler: c.GetPost,
		},
		{
			Method:  http.MethodPut,
			Path:    "/:id",
			Handler: c.UpdatePost,
		},
		{
			Method:  http.MethodDelete,
			Path:    "/:id",
			Handler: c.DeletePost,
		},
	}
}

func NewPostController(postService *service.PostService) *PostController {
	return &PostController{
		postService: postService,
	}
}

func (c *PostController) CreatePost(ctx *gin.Context) {
	var post model.Post
	if err := ctx.ShouldBindJSON(&post); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	createdPost, err := c.postService.CreatePost(post)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, createdPost)
}

func (c *PostController) GetPost(ctx *gin.Context) {
	id := ctx.Param("id")
	post, err := c.postService.GetPostById(id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Post not found"})
		return
	}

	ctx.JSON(http.StatusOK, post)
}

func (c *PostController) UpdatePost(ctx *gin.Context) {
	id := ctx.Param("id")
	var post model.Post
	if err := ctx.ShouldBindJSON(&post); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.postService.UpdatePost(id, post); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

func (c *PostController) DeletePost(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.postService.DeletePost(id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusOK)
}

func (c *PostController) GetPosts(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))
	sortField := ctx.DefaultQuery("sort", "created_at")
	sortDir, _ := strconv.Atoi(ctx.DefaultQuery("direction", "-1"))

	posts, err := c.postService.GetPosts(page, size, ginboot.SortField{
		Field:     sortField,
		Direction: sortDir,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, posts)
}

func (c *PostController) GetPostsByAuthor(ctx *gin.Context) {
	author := ctx.Param("author")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))

	posts, err := c.postService.GetPostsByAuthor(author, page, size)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, posts)
}

func (c *PostController) GetPostsByTags(ctx *gin.Context) {
	tagsStr := ctx.Query("tags")
	tags := strings.Split(tagsStr, ",")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("size", "10"))

	posts, err := c.postService.GetPostsByTags(tags, page, size)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, posts)
}

func (c *PostController) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("", c.GetPosts)
	router.GET("/:id", c.GetPost)
	router.GET("/author/:author", c.GetPostsByAuthor)
	router.GET("/tags", c.GetPostsByTags)
}

func (c *PostController) RegisterProtectedRoutes(router *gin.RouterGroup) {
	router.POST("", c.CreatePost)
	router.PUT("/:id", c.UpdatePost)
	router.DELETE("/:id", c.DeletePost)
}
