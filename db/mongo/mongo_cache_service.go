package mongo

import (
	"context"
	"time"

	"github.com/klass-lk/ginboot"
)

type MongoCacheService struct {
	repo *MongoRepository[ginboot.CacheEntry]
}

func NewMongoCacheService(repo *MongoRepository[ginboot.CacheEntry]) *MongoCacheService {
	return &MongoCacheService{repo: repo}
}

func (s *MongoCacheService) Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error {
	now := time.Now().Unix()
	ttl := time.Now().Add(duration).Unix()

	entry := ginboot.CacheEntry{
		PK:        key, // _id
		Data:      data,
		Tags:      tags,
		TTL:       ttl,
		CreatedAt: now,
	}

	return s.repo.SaveOrUpdate(entry)
}

func (s *MongoCacheService) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := s.repo.FindById(key)
	if err != nil {
		return nil, nil
	}

	if entry.IsExpired() {
		_ = s.repo.Delete(key)
		return nil, nil
	}

	return entry.Data, nil
}

func (s *MongoCacheService) Invalidate(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		_ = s.repo.DeleteBy("tags", tag)
	}
	return nil
}
