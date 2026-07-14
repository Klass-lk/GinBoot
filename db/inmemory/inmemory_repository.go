package inmemory

import (
	"errors"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/klass-lk/ginboot"
)

// InMemoryRepository is an in-memory implementation of ginboot.GenericRepository
type InMemoryRepository[T any] struct {
	store map[string]T
	mutex sync.RWMutex
}

// NewInMemoryRepository creates a new InMemoryRepository
func NewInMemoryRepository[T any]() *InMemoryRepository[T] {
	return &InMemoryRepository[T]{
		store: make(map[string]T),
	}
}

func (r *InMemoryRepository[T]) getGinbootId(entity T) (string, error) {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if tag, ok := field.Tag.Lookup("ginboot"); ok && tag == "id" {
			idVal := val.Field(i).String()
			if idVal == "" {
				return "", errors.New("id field is empty")
			}
			return idVal, nil
		}
	}
	return "", errors.New("no field with `ginboot:\"id\"` tag found")
}

func (r *InMemoryRepository[T]) getFieldValue(entity T, fieldName string) (interface{}, bool) {
	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	typ := val.Type()

	// Try matching by JSON tag first, then by actual field name
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		jsonTag := field.Tag.Get("json")
		jsonTag = strings.Split(jsonTag, ",")[0]

		if jsonTag == fieldName || strings.EqualFold(field.Name, fieldName) {
			return val.Field(i).Interface(), true
		}
	}
	return nil, false
}

func (r *InMemoryRepository[T]) matchesFilters(entity T, filters map[string]interface{}) bool {
	for k, expectedVal := range filters {
		actualVal, found := r.getFieldValue(entity, k)
		if !found {
			return false
		}
		if !reflect.DeepEqual(actualVal, expectedVal) {
			return false
		}
	}
	return true
}

func (r *InMemoryRepository[T]) FindById(id string) (T, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if doc, exists := r.store[id]; exists {
		return doc, nil
	}
	var empty T
	return empty, errors.New("document not found")
}

func (r *InMemoryRepository[T]) FindAllById(ids []string) ([]T, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var result []T
	for _, id := range ids {
		if doc, exists := r.store[id]; exists {
			result = append(result, doc)
		}
	}
	return result, nil
}

func (r *InMemoryRepository[T]) Save(doc T) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	id, err := r.getGinbootId(doc)
	if err != nil {
		return err
	}
	if _, exists := r.store[id]; exists {
		return errors.New("document already exists")
	}
	r.store[id] = doc
	return nil
}

func (r *InMemoryRepository[T]) SaveOrUpdate(doc T) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	id, err := r.getGinbootId(doc)
	if err != nil {
		return err
	}
	r.store[id] = doc
	return nil
}

func (r *InMemoryRepository[T]) SaveAll(docs []T) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, doc := range docs {
		id, err := r.getGinbootId(doc)
		if err != nil {
			return err
		}
		r.store[id] = doc
	}
	return nil
}

func (r *InMemoryRepository[T]) Update(doc T) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	id, err := r.getGinbootId(doc)
	if err != nil {
		return err
	}
	if _, exists := r.store[id]; !exists {
		return errors.New("document not found for update")
	}
	r.store[id] = doc
	return nil
}

func (r *InMemoryRepository[T]) Delete(id string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.store, id)
	return nil
}

func (r *InMemoryRepository[T]) FindOneBy(field string, value interface{}) (T, error) {
	return r.FindOneByFilters(map[string]interface{}{field: value})
}

func (r *InMemoryRepository[T]) FindOneByFilters(filters map[string]interface{}) (T, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, doc := range r.store {
		if r.matchesFilters(doc, filters) {
			return doc, nil
		}
	}
	var empty T
	return empty, errors.New("document not found")
}

func (r *InMemoryRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
	return r.FindByFilters(map[string]interface{}{field: value})
}

func (r *InMemoryRepository[T]) FindByFilters(filters map[string]interface{}) ([]T, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var result []T
	for _, doc := range r.store {
		if r.matchesFilters(doc, filters) {
			result = append(result, doc)
		}
	}
	return result, nil
}

func (r *InMemoryRepository[T]) FindAll(options ...interface{}) ([]T, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var result []T
	for _, doc := range r.store {
		result = append(result, doc)
	}
	return result, nil
}

func (r *InMemoryRepository[T]) applyPagination(docs []T, pageRequest ginboot.PageRequest) ginboot.PageResponse[T] {
	// Handle Sorting
	if pageRequest.Sort.Field != "" {
		sort.SliceStable(docs, func(i, j int) bool {
			valI, okI := r.getFieldValue(docs[i], pageRequest.Sort.Field)
			valJ, okJ := r.getFieldValue(docs[j], pageRequest.Sort.Field)

			if !okI || !okJ {
				return false
			}

			strI := ""
			strJ := ""
			if s, ok := valI.(string); ok {
				strI = s
			}
			if s, ok := valJ.(string); ok {
				strJ = s
			}

			if pageRequest.Sort.Direction < 0 {
				return strI > strJ
			}
			return strI < strJ
		})
	}

	totalElements := len(docs)

	size := pageRequest.Size
	if size <= 0 {
		size = 10
	}

	totalPages := int(math.Ceil(float64(totalElements) / float64(size)))

	page := pageRequest.Page
	if page < 0 {
		page = 0
	}

	start := page * size
	end := start + size

	if start > totalElements {
		start = totalElements
	}
	if end > totalElements {
		end = totalElements
	}

	pagedContent := docs[start:end]
	if pagedContent == nil {
		pagedContent = make([]T, 0)
	}

	return ginboot.PageResponse[T]{
		Contents:         pagedContent,
		NumberOfElements: len(pagedContent),
		Pageable:         pageRequest,
		TotalPages:       totalPages,
		TotalElements:    totalElements,
	}
}

func (r *InMemoryRepository[T]) FindAllPaginated(pageRequest ginboot.PageRequest) (ginboot.PageResponse[T], error) {
	r.mutex.RLock()
	var docs []T
	for _, doc := range r.store {
		docs = append(docs, doc)
	}
	r.mutex.RUnlock()

	return r.applyPagination(docs, pageRequest), nil
}

func (r *InMemoryRepository[T]) FindByPaginated(pageRequest ginboot.PageRequest, filters map[string]interface{}) (ginboot.PageResponse[T], error) {
	r.mutex.RLock()
	var docs []T
	for _, doc := range r.store {
		if r.matchesFilters(doc, filters) {
			docs = append(docs, doc)
		}
	}
	r.mutex.RUnlock()

	return r.applyPagination(docs, pageRequest), nil
}

func (r *InMemoryRepository[T]) CountBy(field string, value interface{}) (int64, error) {
	return r.CountByFilters(map[string]interface{}{field: value})
}

func (r *InMemoryRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	var count int64 = 0
	for _, doc := range r.store {
		if r.matchesFilters(doc, filters) {
			count++
		}
	}
	return count, nil
}

func (r *InMemoryRepository[T]) ExistsBy(field string, value interface{}) (bool, error) {
	return r.ExistsByFilters(map[string]interface{}{field: value})
}

func (r *InMemoryRepository[T]) ExistsByFilters(filters map[string]interface{}) (bool, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	for _, doc := range r.store {
		if r.matchesFilters(doc, filters) {
			return true, nil
		}
	}
	return false, nil
}

func (r *InMemoryRepository[T]) DeleteAll(options ...interface{}) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.store = make(map[string]T)
	return nil
}
