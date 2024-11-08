package GinBoot

import (
	"github.com/klass-lk/ginboot/types"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type GenericRepository[T types.Document] interface {
	Query() *mongo.Collection
	FindById(id interface{}) (T, error)
	FindAllById(idList []string) ([]T, error)
	Save(doc T) error
	SaveOrUpdate(doc T) error
	SaveAll(sms []T) error
	Update(doc T) error
	Delete(id string) error
	FindOneBy(field string, value interface{}) (T, error)
	FindOneByFilters(filters map[string]interface{}) (T, error)
	FindBy(field string, value interface{}) ([]T, error)
	FindByMultiple(filters map[string]interface{}) ([]T, error)
	FindAll(opts ...*options.FindOptions) ([]T, error)
	FindAllPaginated(pageRequest types.PageRequest) (types.PageResponse[T], error)
	FindByPaginated(pageRequest types.PageRequest, filters map[string]interface{}) (types.PageResponse[T], error)
	CountBy(field string, value interface{}) (int64, error)
	CountByFilters(filters map[string]interface{}) (int64, error)
	ExistsBy(field string, value interface{}) (bool, error)
	ExistsByFilters(filters map[string]interface{}) (bool, error)
}
