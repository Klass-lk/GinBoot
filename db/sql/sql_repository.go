package sql

import (
	"fmt"

	"github.com/klass-lk/ginboot"
	"gorm.io/gorm"
)

type SQLRepository[T any] struct {
	db        *gorm.DB
	tableName string
}

func NewSQLRepository[T any](db *gorm.DB) *SQLRepository[T] {
	repo := &SQLRepository[T]{
		db: db,
	}
	var doc T
	if d, ok := any(doc).(ginboot.Document); ok {
		repo.tableName = d.GetTableName()
	}
	return repo
}

func (r *SQLRepository[T]) scopedDB() *gorm.DB {
	if r.tableName != "" {
		return r.db.Table(r.tableName)
	}
	var entity T
	return r.db.Model(&entity)
}

func (r *SQLRepository[T]) FindById(id string) (T, error) {
	var result T
	err := r.scopedDB().Where("id = ?", id).First(&result).Error
	return result, err
}

func (r *SQLRepository[T]) FindAllById(ids []string) ([]T, error) {
	if len(ids) == 0 {
		return []T{}, nil
	}
	var results []T
	err := r.scopedDB().Where("id IN ?", ids).Find(&results).Error
	return results, err
}

func (r *SQLRepository[T]) Save(doc T) error {
	return r.scopedDB().Create(&doc).Error
}

func (r *SQLRepository[T]) SaveOrUpdate(doc T) error {
	return r.scopedDB().Save(&doc).Error
}

func (r *SQLRepository[T]) SaveAll(docs []T) error {
	if len(docs) == 0 {
		return nil
	}
	return r.scopedDB().Create(&docs).Error
}

func (r *SQLRepository[T]) Update(doc T) error {
	return r.scopedDB().Save(&doc).Error
}

func (r *SQLRepository[T]) Delete(id string) error {
	var entity T
	return r.scopedDB().Where("id = ?", id).Delete(&entity).Error
}

func (r *SQLRepository[T]) FindOneBy(field string, value interface{}) (T, error) {
	var result T
	err := r.scopedDB().Where(fmt.Sprintf("%s = ?", field), value).First(&result).Error
	return result, err
}

func (r *SQLRepository[T]) FindOneByFilters(filters map[string]interface{}) (T, error) {
	var result T
	err := r.scopedDB().Where(filters).First(&result).Error
	return result, err
}

func (r *SQLRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
	var results []T
	err := r.scopedDB().Where(fmt.Sprintf("%s = ?", field), value).Find(&results).Error
	return results, err
}

func (r *SQLRepository[T]) FindByFilters(filters map[string]interface{}) ([]T, error) {
	var results []T
	err := r.scopedDB().Where(filters).Find(&results).Error
	return results, err
}

func (r *SQLRepository[T]) FindAll(options ...interface{}) ([]T, error) {
	var results []T
	tx := r.scopedDB()
	for _, opt := range options {
		tx = tx.Where(opt)
	}
	err := tx.Find(&results).Error
	return results, err
}

func (r *SQLRepository[T]) FindAllPaginated(pageRequest ginboot.PageRequest) (ginboot.PageResponse[T], error) {
	var results []T
	var total int64

	page := pageRequest.Page
	if page < 1 {
		page = 1
	}
	size := pageRequest.Size
	if size <= 0 {
		size = 10
	}
	offset := (page - 1) * size

	tx := r.scopedDB()
	var entity T
	if err := tx.Model(&entity).Count(&total).Error; err != nil {
		return ginboot.PageResponse[T]{}, err
	}

	if pageRequest.Sort.Field != "" {
		dir := "ASC"
		if pageRequest.Sort.Direction < 0 {
			dir = "DESC"
		}
		tx = tx.Order(fmt.Sprintf("%s %s", pageRequest.Sort.Field, dir))
	}

	if err := tx.Offset(offset).Limit(size).Find(&results).Error; err != nil {
		return ginboot.PageResponse[T]{}, err
	}

	totalPages := int((total + int64(size) - 1) / int64(size))

	return ginboot.PageResponse[T]{
		Contents:         results,
		NumberOfElements: len(results),
		Pageable:         pageRequest,
		TotalElements:    int(total),
		TotalPages:       totalPages,
	}, nil
}

func (r *SQLRepository[T]) FindByPaginated(pageRequest ginboot.PageRequest, filters map[string]interface{}) (ginboot.PageResponse[T], error) {
	var results []T
	var total int64

	page := pageRequest.Page
	if page < 1 {
		page = 1
	}
	size := pageRequest.Size
	if size <= 0 {
		size = 10
	}
	offset := (page - 1) * size

	tx := r.scopedDB().Where(filters)
	var entity T
	if err := tx.Model(&entity).Count(&total).Error; err != nil {
		return ginboot.PageResponse[T]{}, err
	}

	if pageRequest.Sort.Field != "" {
		dir := "ASC"
		if pageRequest.Sort.Direction < 0 {
			dir = "DESC"
		}
		tx = tx.Order(fmt.Sprintf("%s %s", pageRequest.Sort.Field, dir))
	}

	if err := tx.Offset(offset).Limit(size).Find(&results).Error; err != nil {
		return ginboot.PageResponse[T]{}, err
	}

	totalPages := int((total + int64(size) - 1) / int64(size))

	return ginboot.PageResponse[T]{
		Contents:         results,
		NumberOfElements: len(results),
		Pageable:         pageRequest,
		TotalElements:    int(total),
		TotalPages:       totalPages,
	}, nil
}

func (r *SQLRepository[T]) CountBy(field string, value interface{}) (int64, error) {
	var count int64
	var entity T
	err := r.scopedDB().Model(&entity).Where(fmt.Sprintf("%s = ?", field), value).Count(&count).Error
	return count, err
}

func (r *SQLRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
	var count int64
	var entity T
	err := r.scopedDB().Model(&entity).Where(filters).Count(&count).Error
	return count, err
}

func (r *SQLRepository[T]) ExistsBy(field string, value interface{}) (bool, error) {
	count, err := r.CountBy(field, value)
	return count > 0, err
}

func (r *SQLRepository[T]) ExistsByFilters(filters map[string]interface{}) (bool, error) {
	count, err := r.CountByFilters(filters)
	return count > 0, err
}

func (r *SQLRepository[T]) DeleteAll(options ...interface{}) error {
	var entity T
	tx := r.scopedDB()
	for _, opt := range options {
		tx = tx.Where(opt)
	}
	return tx.Where("1=1").Delete(&entity).Error
}

func (r *SQLRepository[T]) DeleteBy(field string, value interface{}) error {
	var entity T
	return r.scopedDB().Where(fmt.Sprintf("%s = ?", field), value).Delete(&entity).Error
}

func (r *SQLRepository[T]) DeleteByFilters(filters map[string]interface{}) error {
	var entity T
	return r.scopedDB().Where(filters).Delete(&entity).Error
}

func (r *SQLRepository[T]) CreateTable() error {
	var entity T
	if r.tableName != "" {
		return r.db.Table(r.tableName).AutoMigrate(&entity)
	}
	return r.db.AutoMigrate(&entity)
}
