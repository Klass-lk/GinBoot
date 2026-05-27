package sql

import (
	"context"
	"fmt"
	"time"

	"github.com/klass-lk/ginboot"
)

type SQLCacheService struct {
	cacheRepo *SQLRepository[ginboot.CacheEntry]
	tagRepo   *SQLRepository[ginboot.TagEntry]
}

func NewSQLCacheService(cRepo *SQLRepository[ginboot.CacheEntry], tRepo *SQLRepository[ginboot.TagEntry]) *SQLCacheService {
	_ = cRepo.CreateTable()
	_ = tRepo.CreateTable()
	return &SQLCacheService{
		cacheRepo: cRepo,
		tagRepo:   tRepo,
	}
}

func (s *SQLCacheService) Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error {
	now := time.Now().Unix()
	ttl := time.Now().Add(duration).Unix()

	entry := ginboot.CacheEntry{
		PK:        key, // In SQL this maps to 'id' column
		Data:      data,
		TTL:       ttl,
		CreatedAt: now,
	}

	if err := s.cacheRepo.SaveOrUpdate(entry); err != nil {
		return err
	}

	_ = s.tagRepo.DeleteBy("cache_key", key)

	var tagEntries []ginboot.TagEntry
	for _, tag := range tags {
		tagEntries = append(tagEntries, ginboot.TagEntry{
			ID:        fmt.Sprintf("%s:%s", tag, key), // Composite ID
			Tag:       tag,
			CacheKey:  key,
			TTL:       ttl,
			CreatedAt: now,
		})
	}

	if len(tagEntries) > 0 {
		return s.tagRepo.SaveAll(tagEntries)
	}
	return nil
}

func (s *SQLCacheService) Get(ctx context.Context, key string) ([]byte, error) {
	entry, err := s.cacheRepo.FindById(key)
	if err != nil {
		return nil, nil // SQL row not found
	}

	if entry.IsExpired() {
		_ = s.cacheRepo.Delete(key)
		return nil, nil
	}

	return entry.Data, nil
}

func (s *SQLCacheService) Invalidate(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		// 1. Find tags
		tagEntries, err := s.tagRepo.FindBy("tag", tag)
		if err != nil {
			continue
		}

		// 2. Delete associated cache entries
		for _, te := range tagEntries {
			_ = s.cacheRepo.Delete(te.CacheKey)
		}

		// 3. Delete tag entries
		_ = s.tagRepo.DeleteBy("tag", tag)
	}
	return nil
}
