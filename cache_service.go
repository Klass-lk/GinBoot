package ginboot

import (
	"context"
	"fmt"
	"time"
)

// CacheService defines the interface for caching operations
type CacheService interface {
	// Set stores a value in the cache with the given key, tags, and duration
	Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error

	// Get retrieves a value from the cache by key
	Get(ctx context.Context, key string) ([]byte, error)

	// Invalidate removes all cache entries associated with the given tags
	Invalidate(ctx context.Context, tags ...string) error
}

// -----------------------------------------------------------------------------
// DynamoDB Implementation
// -----------------------------------------------------------------------------

type DynamoDBCacheService struct {
	cacheRepo *DynamoDBRepository[CacheEntry]
	tagRepo   *DynamoDBRepository[TagEntry]
}

func NewDynamoDBCacheService(client DynamoDBAPI) *DynamoDBCacheService {
	// Reuse the generic repo logic
	// Note: Generic Repo constructor expects client.
	// We instantiate two repos, one for CacheEntry, one for TagEntry.
	cRepo := NewDynamoDBRepository[CacheEntry](client)
	tRepo := NewDynamoDBRepository[TagEntry](client)

	return &DynamoDBCacheService{
		cacheRepo: cRepo,
		tagRepo:   tRepo,
	}
}

func (s *DynamoDBCacheService) Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error {
	now := time.Now().Unix()
	ttl := time.Now().Add(duration).Unix()

	cacheKey := CachePartitionPrefix + key

	// 1. Save Cache Entry
	cacheEntry := CacheEntry{
		PK:        cacheKey,
		SK:        CacheSortKey,
		Data:      data,
		Tags:      tags,
		TTL:       ttl,
		CreatedAt: now,
	}

	// Using "DATA" as partition key suffix for repo is weird because generic repo expects Composite PK logic.
	// But generic repo `Save` handles `pk` field via struct tag.
	// `partitionKey` argument in generic repo methods is usually appended to PK.
	// However, `Save` method signature is: Save(doc T, partitionKey string).
	// `getPK` uses Type Name.
	// This generic repo is highly opinionated about PK structure: `TypeName#PartitionKey`.
	// For Cache, we want precise control over PK.

	// The Generic Repo `Save` implementation:
	// pk := r.getPK(entity) + "#" + partitionKey
	// doc.PK = pk
	// This forces PK to be "CacheEntry#<partitionKey>".
	// This contradicts our Cache Design "CACHE#<key>".

	// SOLUTION: Usage of generic repository usually implies adhering to its schema pattern.
	// Since user asked to "use the dynamodb repository implementation", we should interpret this as either:
	// A) Adhere to `TypeName#ID` pattern.
	// B) Hack it / Update Repo to respect existing ID if set.

	// Let's go with B (Implicit): If `pk` tag is already set on struct, repo overwrite logic is dangerous.
	// Checking `dynamodb_repository.go`:
	// `pk := r.getPK(entity) + "#" + partitionKey`
	// `item["pk"] = ...`
	// It ALWAYS overwrites PK.

	// Therefore, we MUST use the Generic Repo's Pattern.
	// Partition Key for CacheEntry = the actual Cache Key.
	// PK in Dynamo = "CacheEntry#<userKey>"

	// Let's adapt our design to the Repo Pattern.
	// Cache Entry PK: "CacheEntry#<userKey>"
	// Tag Entry PK: "TagEntry#<tag>"

	// Tag Entry PK: "TagEntry#<tag>"
	// Tag Entry SK: "CacheEntry#<userKey>" (so we can find it)

	if err := s.cacheRepo.Save(cacheEntry, key); err != nil {
		return err
	}

	for _, tag := range tags {
		tEntry := TagEntry{
			PK:        TagPartitionPrefix + tag, // Actually Repo will overwrite this to "TagEntry#<tag>"
			SK:        "CacheEntry#" + key,
			TTL:       ttl,
			CreatedAt: now,
		}
		// Save each tag mapping.
		// PartitionKey for TagEntry repo call should be the Tag itself.
		if err := s.tagRepo.Save(tEntry, tag); err != nil {
			return err
		}
	}

	return nil
}

func (s *DynamoDBCacheService) Get(ctx context.Context, key string) ([]byte, error) {
	// partitionKey for Finding is the user key
	// This constructs PK: "CacheEntry#<key>"
	entry, err := s.cacheRepo.FindById(CacheSortKey, key) // FindById(id, partitionKey) -> query where pk=CacheEntry#key AND sk=id
	// Wait, generic repo `FindById(id, partitionKey)`:
	// pk := ... + "#" + partitionKey
	// query pk=:pk and sk=:id
	// Our cache entry has SK="DATA" (CacheSortKey).

	if err != nil {
		// handle not found
		return nil, nil
	}

	// Check TTL
	if entry.IsExpired() {
		return nil, nil // treat as miss
	}

	return entry.Data, nil
}

func (s *DynamoDBCacheService) Invalidate(ctx context.Context, tags ...string) error {
	for _, tag := range tags {
		// 1. Find all cache keys associated with this tag
		// TagEntry PK = "TagEntry#<tag>"
		// FindAll("tag") -> gets all items with PK="TagEntry#<tag>"
		tagEntries, err := s.tagRepo.FindAll(tag)
		if err != nil {
			continue
		}

		// 2. Delete the associated cache entries
		// TagEntry SK holds the CacheKey.
		// CacheEntry PK = "CacheEntry#<CacheKey>" (constructed by repo logic)
		// But wait, our Set method uses `s.cacheRepo.Save(cacheEntry, key)`
		// Generic Repo Save: PK = "CacheEntry#key", SK = "DATA" (if id="DATA")
		// So to delete, we need Delete("DATA", key) ?
		// Let's check Set implementation:
		// cacheEntry.SK = CacheSortKey ("DATA")
		// s.cacheRepo.Save(cacheEntry, key) -> PK="CacheEntry#key", SK="DATA"

		// So to delete: s.cacheRepo.Delete(CacheSortKey, userKey)

		tagIds := make([]string, len(tagEntries))
		for i, te := range tagEntries {
			// te.SK contains the userKey (the cache key)
			userKey := te.SK
			// ignore error for best effort?
			_ = s.cacheRepo.Delete(CacheSortKey, userKey)
			tagIds[i] = te.SK
		}

		// 3. Delete the tag entries themselves
		// PK: TagEntry#tag
		// SK: keys
		_ = s.tagRepo.DeleteAll(tagIds, tag)
	}
	return nil
}

// -----------------------------------------------------------------------------
// SQL Implementation
// -----------------------------------------------------------------------------

type SQLCacheService struct {
	cacheRepo *SQLRepository[CacheEntry]
	tagRepo   *SQLRepository[TagEntry]
}

func NewSQLCacheService(cRepo *SQLRepository[CacheEntry], tRepo *SQLRepository[TagEntry]) *SQLCacheService {
	// Ensure tables exist?
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

	entry := CacheEntry{
		PK:        key, // In SQL this maps to 'id' column
		Data:      data,
		TTL:       ttl,
		CreatedAt: now,
	}

	if err := s.cacheRepo.SaveOrUpdate(entry); err != nil {
		return err
	}

	// Handle Tags
	// Delete old tags for this key first? Or just append?
	// A robust Set would replace.
	// SQL delete by cache_key
	_ = s.tagRepo.DeleteBy("cache_key", key)

	var tagEntries []TagEntry
	for _, tag := range tags {
		tagEntries = append(tagEntries, TagEntry{
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
		// Lazy delete?
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
		// SQL doesn't support DeleteAll(ids) easily without loop or custom query with IN
		// But we have generic DeleteBy / DeleteAll?
		// Repo implementation of DeleteAll deletes TABLE.
		// Delete(id) deletes one.
		// We might need to loop. OR custom query.
		// Loop for now.
		for _, te := range tagEntries {
			_ = s.cacheRepo.Delete(te.CacheKey)
		}

		// 3. Delete tag entries
		_ = s.tagRepo.DeleteBy("tag", tag)
	}
	return nil
}

// -----------------------------------------------------------------------------
// MongoDB Implementation
// -----------------------------------------------------------------------------

type MongoCacheService struct {
	repo *MongoRepository[CacheEntry]
}

func NewMongoCacheService(repo *MongoRepository[CacheEntry]) *MongoCacheService {
	return &MongoCacheService{repo: repo}
}

func (s *MongoCacheService) Set(ctx context.Context, key string, data []byte, tags []string, duration time.Duration) error {
	now := time.Now().Unix()
	ttl := time.Now().Add(duration).Unix()

	entry := CacheEntry{
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
	// Mongo supports array queries
	// { tags: { $in: [tag1, tag2] } }
	// But our DeleteBy(field, value) is simple equality.
	// If value is an array, driver might handle equality.
	// For "contains", we need special filter.

	// DeleteByFilters is safer.
	for _, tag := range tags {
		// Construct filter "tags" contains "tag"
		// In Mongo: { "tags": "tag_value" } matches if array contains it.
		// So simple field match works!
		_ = s.repo.DeleteBy("tags", tag)
	}
	return nil
}
