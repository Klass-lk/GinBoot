package dynamodb

import (
	"context"
	"time"

	"github.com/klass-lk/ginboot"
)

type DynamoDBCacheService struct {
	cacheRepo *DynamoDBRepository[ginboot.CacheEntry]
	tagRepo   *DynamoDBRepository[ginboot.TagEntry]
}

func NewDynamoDBCacheService(client DynamoDBAPI) *DynamoDBCacheService {
	cRepo := NewDynamoDBRepository[ginboot.CacheEntry](client)
	tRepo := NewDynamoDBRepository[ginboot.TagEntry](client)

	return &DynamoDBCacheService{
		cacheRepo: cRepo,
		tagRepo:   tRepo,
	}
}

func (s *DynamoDBCacheService) Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error {
	now := time.Now().Unix()
	ttl := time.Now().Add(duration).Unix()

	cacheKey := ginboot.CachePartitionPrefix + key

	cacheEntry := ginboot.CacheEntry{
		PK:        cacheKey,
		SK:        ginboot.CacheSortKey,
		Data:      data,
		Tags:      tags,
		TTL:       ttl,
		CreatedAt: now,
	}

	if err := s.cacheRepo.Save(cacheEntry, key); err != nil {
		return err
	}

	for _, tag := range tags {
		tEntry := ginboot.TagEntry{
			PK:        ginboot.TagPartitionPrefix + tag,
			SK:        "CacheEntry#" + key,
			TTL:       ttl,
			CreatedAt: now,
		}
		if err := s.tagRepo.Save(tEntry, tag); err != nil {
			return err
		}
	}

	return nil
}

func (s *DynamoDBCacheService) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := s.cacheRepo.FindById(ginboot.CacheSortKey, key)
	if err != nil {
		return nil, nil
	}

	if entry.IsExpired() {
		return nil, nil
	}

	return entry.Data, nil
}

func (s *DynamoDBCacheService) Invalidate(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		tagEntries, err := s.tagRepo.FindAll(tag)
		if err != nil {
			continue
		}

		tagIds := make([]string, len(tagEntries))
		for i, te := range tagEntries {
			userKey := te.SK
			_ = s.cacheRepo.Delete(ginboot.CacheSortKey, userKey)
			tagIds[i] = te.SK
		}

		_ = s.tagRepo.DeleteAll(tagIds, tag)
	}
	return nil
}
