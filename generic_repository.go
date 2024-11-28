package ginboot

type GenericRepository[T Document] interface {
	FindById(id interface{}) (T, error)
	FindAllById(ids []interface{}) ([]T, error)
	Save(doc T) error
	SaveOrUpdate(doc T) error
	SaveAll(docs []T) error
	Update(doc T) error
	Delete(id interface{}) error
	FindOneBy(field string, value interface{}) (T, error)
	FindOneByFilters(filters map[string]interface{}) (T, error)
	FindBy(field string, value interface{}) ([]T, error)
	FindByFilters(filters map[string]interface{}) ([]T, error)
	FindAll(options ...interface{}) ([]T, error)
	FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error)
	FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error)
	CountBy(field string, value interface{}) (int64, error)
	CountByFilters(filters map[string]interface{}) (int64, error)
	ExistsBy(field string, value interface{}) (bool, error)
	ExistsByFilters(filters map[string]interface{}) (bool, error)
}
