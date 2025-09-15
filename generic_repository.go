package ginboot

// GenericRepository defines the interface for a generic repository with string IDs
type GenericRepository[T any] interface {
	// FindById finds a document by its string ID
	FindById(id string) (T, error)

	// FindAllById finds all documents with the given string IDs
	FindAllById(ids []string) ([]T, error)

	// Save saves a document
	Save(doc T) error

	// SaveOrUpdate saves or updates a document
	SaveOrUpdate(doc T) error

	// SaveAll saves multiple documents
	SaveAll(docs []T) error

	// Update updates an existing document
	Update(doc T) error

	// Delete deletes a document by its string ID
	Delete(id string) error

	// FindOneBy finds a document by a field value
	FindOneBy(field string, value interface{}) (T, error)

	// FindOneByFilters finds a document by multiple filters
	FindOneByFilters(filters map[string]interface{}) (T, error)

	// FindBy finds documents by a field value
	FindBy(field string, value interface{}) ([]T, error)

	// FindByFilters finds documents by multiple filters
	FindByFilters(filters map[string]interface{}) ([]T, error)

	// FindAll finds all documents
	FindAll(options ...interface{}) ([]T, error)

	// FindAllPaginated finds all documents with pagination
	FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error)

	// FindByPaginated finds documents by filters with pagination
	FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error)

	// CountBy counts documents by a field value
	CountBy(field string, value interface{}) (int64, error)

	// CountByFilters counts documents by multiple filters
	CountByFilters(filters map[string]interface{}) (int64, error)

	// ExistsBy checks if a document exists by a field value
	ExistsBy(field string, value interface{}) (bool, error)

	// ExistsByFilters checks if a document exists by multiple filters
	ExistsByFilters(filters map[string]interface{}) (bool, error)

	// DeleteAll deletes all documents
	DeleteAll(options ...interface{}) error
}
