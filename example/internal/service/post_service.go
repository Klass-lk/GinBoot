package service

import (
	"time"

	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/example/internal/model"
	"github.com/klass-lk/ginboot/example/internal/repository"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type PostService struct {
	postRepo *repository.PostRepository
}

func NewPostService(postRepo *repository.PostRepository) *PostService {
	return &PostService{
		postRepo: postRepo,
	}
}

func (s *PostService) CreatePost(post model.Post) (model.Post, error) {
	post.ID = primitive.NewObjectID()
	post.CreatedAt = time.Now()
	post.UpdatedAt = time.Now()

	err := s.postRepo.Save(post)
	return post, err
}

func (s *PostService) GetPostById(id string) (model.Post, error) {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return model.Post{}, err
	}
	return s.postRepo.FindById(objectId)
}

func (s *PostService) UpdatePost(id string, post model.Post) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	existingPost, err := s.postRepo.FindById(objectId)
	if err != nil {
		return err
	}

	post.ID = existingPost.ID
	post.CreatedAt = existingPost.CreatedAt
	post.UpdatedAt = time.Now()

	return s.postRepo.Update(post)
}

func (s *PostService) DeletePost(id string) error {
	objectId, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	return s.postRepo.Delete(objectId)
}

func (s *PostService) GetPosts(page, size int, sort ginboot.SortField) (ginboot.PageResponse[model.Post], error) {
	return s.postRepo.FindAllPaginated(ginboot.PageRequest{
		Page: page,
		Size: size,
		Sort: sort,
	})
}

func (s *PostService) GetPostsByAuthor(author string, page, size int) (ginboot.PageResponse[model.Post], error) {
	return s.postRepo.FindByPaginated(
		ginboot.PageRequest{
			Page: page,
			Size: size,
		},
		map[string]interface{}{
			"author": author,
		},
	)
}

func (s *PostService) GetPostsByTags(tags []string, page, size int) (ginboot.PageResponse[model.Post], error) {
	return s.postRepo.FindByPaginated(
		ginboot.PageRequest{
			Page: page,
			Size: size,
		},
		map[string]interface{}{
			"tags": map[string]interface{}{
				"$in": tags,
			},
		},
	)
}
