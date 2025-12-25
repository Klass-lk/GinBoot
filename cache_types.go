package ginboot

import "time"

// CacheEntry represents the main cache item stored in DynamoDB/SQL/Mongo
type CacheEntry struct {
	// Common Fields
	PK   string `dynamodbav:"pk" json:"pk" bson:"_id" db:"id" ginboot:"id"`
	Data []byte `dynamodbav:"data" json:"data" bson:"data" db:"data"`

	// DynamoDB Specific
	SK string `dynamodbav:"sk" json:"sk,omitempty" bson:"-" db:"-"`

	// Metadata
	TTL       int64 `dynamodbav:"ttl" json:"ttl" bson:"ttl" db:"ttl"`
	CreatedAt int64 `dynamodbav:"createdAt" json:"createdAt" bson:"createdAt" db:"created_at"`

	// Mongo Specific (Embedded tags for querying)
	Tags []string `dynamodbav:"tags,omitempty" json:"tags,omitempty" bson:"tags,omitempty" db:"-"`
}

func (c CacheEntry) GetTableName() string {
	return "cache_entries"
}

// TagEntry is used primarily for DynamoDB Inverted Index pattern or SQL join table simulation.
// For MongoDB, we just query the Tags array in CacheEntry directly.
// For SQL, this maps to 'cache_tags' table.
type TagEntry struct {
	ID        string `dynamodbav:"-" json:"id" bson:"_id" db:"id" ginboot:"id"` // Composite ID for SQL: "tag:key"
	PK        string `dynamodbav:"pk" json:"pk" bson:"pk"`                      // TAG#<tag>
	SK        string `dynamodbav:"sk" json:"sk" bson:"sk"`                      // CACHE#<key>
	TTL       int64  `dynamodbav:"ttl" json:"ttl" bson:"ttl" db:"ttl"`
	CreatedAt int64  `dynamodbav:"createdAt" json:"createdAt" bson:"createdAt" db:"created_at"`

	// Helper fields for SQL
	Tag      string `dynamodbav:"-" json:"-" bson:"-" db:"tag"`
	CacheKey string `dynamodbav:"-" json:"-" bson:"-" db:"cache_key"`
}

func (t TagEntry) GetTableName() string {
	return "cache_tags"
}

const (
	CachePartitionPrefix = "CACHE#"
	TagPartitionPrefix   = "TAG#"
	CacheSortKey         = "DATA"
)

// Helper to check if cache is expired
func (e *CacheEntry) IsExpired() bool {
	return time.Now().Unix() > e.TTL
}
