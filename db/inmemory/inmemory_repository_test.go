package inmemory

import (
	"github.com/klass-lk/ginboot"
	"testing"
)

type TestEntity struct {
	ID    string `ginboot:"id"`
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestInMemoryRepository_CRUD(t *testing.T) {
	repo := NewInMemoryRepository[TestEntity]()

	entity := TestEntity{ID: "1", Name: "Test 1", Value: 100}

	// Test Save
	err := repo.Save(entity)
	if err != nil {
		t.Fatalf("expected no error on Save, got %v", err)
	}

	// Test Save duplicate
	err = repo.Save(entity)
	if err == nil {
		t.Fatalf("expected error on duplicate Save, got nil")
	}

	// Test FindById
	found, err := repo.FindById("1")
	if err != nil {
		t.Fatalf("expected no error on FindById, got %v", err)
	}
	if found.Name != "Test 1" {
		t.Fatalf("expected Name 'Test 1', got '%s'", found.Name)
	}

	// Test Update
	entity.Name = "Updated Test 1"
	err = repo.Update(entity)
	if err != nil {
		t.Fatalf("expected no error on Update, got %v", err)
	}

	found, _ = repo.FindById("1")
	if found.Name != "Updated Test 1" {
		t.Fatalf("expected updated Name, got '%s'", found.Name)
	}

	// Test Delete
	err = repo.Delete("1")
	if err != nil {
		t.Fatalf("expected no error on Delete, got %v", err)
	}

	_, err = repo.FindById("1")
	if err == nil {
		t.Fatalf("expected error on FindById after Delete, got nil")
	}
}

func TestInMemoryRepository_FiltersAndPagination(t *testing.T) {
	repo := NewInMemoryRepository[TestEntity]()

	repo.Save(TestEntity{ID: "1", Name: "A", Value: 10})
	repo.Save(TestEntity{ID: "2", Name: "B", Value: 20})
	repo.Save(TestEntity{ID: "3", Name: "A", Value: 30})

	// Test FindBy
	results, err := repo.FindBy("name", "A")
	if err != nil {
		t.Fatalf("expected no error on FindBy, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Test FindByFilters
	results, err = repo.FindByFilters(map[string]interface{}{"name": "A", "value": 30})
	if err != nil || len(results) != 1 {
		t.Fatalf("expected 1 result, got %d (err: %v)", len(results), err)
	}

	// Test FindAllPaginated
	pageRes, err := repo.FindAllPaginated(ginboot.PageRequest{
		Page: 0,
		Size: 2,
		Sort: ginboot.SortField{Field: "value", Direction: 1}, // Ascending
	})

	if err != nil {
		t.Fatalf("expected no error on FindAllPaginated, got %v", err)
	}
	if len(pageRes.Contents) != 2 {
		t.Fatalf("expected 2 items in page, got %d", len(pageRes.Contents))
	}
	if pageRes.TotalElements != 3 {
		t.Fatalf("expected 3 total elements, got %d", pageRes.TotalElements)
	}
	if pageRes.TotalPages != 2 {
		t.Fatalf("expected 2 total pages, got %d", pageRes.TotalPages)
	}
}

func TestInMemoryRepository_Pointers(t *testing.T) {
	repo := NewInMemoryRepository[*TestEntity]()

	entity := &TestEntity{ID: "ptr1", Name: "Ptr Test", Value: 100}

	err := repo.Save(entity)
	if err != nil {
		t.Fatalf("expected no error on ptr Save, got %v", err)
	}

	found, err := repo.FindById("ptr1")
	if err != nil {
		t.Fatalf("expected no error on ptr FindById, got %v", err)
	}
	if found.Name != "Ptr Test" {
		t.Fatalf("expected Name 'Ptr Test', got '%s'", found.Name)
	}

	// Test update pointer
	entity.Name = "Updated Ptr"
	repo.Update(entity)

	found, _ = repo.FindById("ptr1")
	if found.Name != "Updated Ptr" {
		t.Fatalf("expected 'Updated Ptr', got '%s'", found.Name)
	}
}

func TestInMemoryRepository_EdgeCases(t *testing.T) {
	repo := NewInMemoryRepository[TestEntity]()

	// 1. Missing ID tag
	type BadEntity struct {
		Name string
	}
	badRepo := NewInMemoryRepository[BadEntity]()
	err := badRepo.Save(BadEntity{Name: "test"})
	if err == nil || err.Error() != "no field with `ginboot:\"id\"` tag found" {
		t.Fatalf("expected no id tag error, got %v", err)
	}

	// 2. Empty ID string
	err = repo.Save(TestEntity{ID: "", Name: "empty"})
	if err == nil || err.Error() != "id field is empty" {
		t.Fatalf("expected empty id error, got %v", err)
	}

	// 3. FindById not found
	_, err = repo.FindById("not-exist")
	if err == nil || err.Error() != "document not found" {
		t.Fatalf("expected not found error, got %v", err)
	}

	// 4. FindOneBy not found
	_, err = repo.FindOneBy("name", "not-exist")
	if err == nil || err.Error() != "document not found" {
		t.Fatalf("expected not found error, got %v", err)
	}

	// 5. Update not found
	err = repo.Update(TestEntity{ID: "not-exist"})
	if err == nil || err.Error() != "document not found for update" {
		t.Fatalf("expected not found for update, got %v", err)
	}

	// 6. Delete not found (should not error)
	err = repo.Delete("not-exist")
	if err != nil {
		t.Fatalf("expected no error on delete non-existent, got %v", err)
	}
}

func TestInMemoryRepository_BulkAndAggregate(t *testing.T) {
	repo := NewInMemoryRepository[TestEntity]()

	entities := []TestEntity{
		{ID: "1", Name: "Bulk 1", Value: 1},
		{ID: "2", Name: "Bulk 2", Value: 2},
		{ID: "3", Name: "Bulk 3", Value: 1}, // same value as 1
	}

	// SaveAll
	err := repo.SaveAll(entities)
	if err != nil {
		t.Fatalf("expected no error on SaveAll, got %v", err)
	}

	// FindAll
	all, err := repo.FindAll()
	if err != nil || len(all) != 3 {
		t.Fatalf("expected 3 results from FindAll, got %d", len(all))
	}

	// FindAllById
	found, err := repo.FindAllById([]string{"1", "3", "not-exist"})
	if err != nil || len(found) != 2 {
		t.Fatalf("expected 2 results from FindAllById, got %d", len(found))
	}

	// CountBy
	count, err := repo.CountBy("value", 1)
	if err != nil || count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	// ExistsBy
	exists, err := repo.ExistsBy("value", 2)
	if err != nil || !exists {
		t.Fatalf("expected exists true, got %v", exists)
	}
	exists, _ = repo.ExistsBy("value", 999)
	if exists {
		t.Fatalf("expected exists false for 999")
	}

	// SaveOrUpdate (Insert)
	repo.SaveOrUpdate(TestEntity{ID: "4", Name: "New", Value: 4})
	if c, _ := repo.CountByFilters(nil); c != 4 {
		t.Fatalf("expected 4 items after SaveOrUpdate insert")
	}

	// SaveOrUpdate (Update)
	repo.SaveOrUpdate(TestEntity{ID: "4", Name: "Updated New", Value: 40})
	doc, _ := repo.FindById("4")
	if doc.Name != "Updated New" {
		t.Fatalf("expected SaveOrUpdate to update existing item")
	}

	// DeleteAll
	err = repo.DeleteAll()
	if err != nil {
		t.Fatalf("expected no error on DeleteAll, got %v", err)
	}
	if all, _ := repo.FindAll(); len(all) != 0 {
		t.Fatalf("expected empty store after DeleteAll")
	}
}

func TestInMemoryRepository_SortingBounds(t *testing.T) {
	repo := NewInMemoryRepository[TestEntity]()

	repo.Save(TestEntity{ID: "C", Name: "Charlie", Value: 3})
	repo.Save(TestEntity{ID: "A", Name: "Alpha", Value: 1})
	repo.Save(TestEntity{ID: "B", Name: "Bravo", Value: 2})

	// Reverse sort string (Name)
	pageRes, _ := repo.FindAllPaginated(ginboot.PageRequest{
		Page: 0,
		Size: 10,
		Sort: ginboot.SortField{Field: "Name", Direction: -1}, // Descending
	})

	if len(pageRes.Contents) != 3 || pageRes.Contents[0].Name != "Charlie" {
		t.Fatalf("expected Charlie first in descending sort, got %v", pageRes.Contents[0].Name)
	}

	// Missing field sort
	pageRes, _ = repo.FindAllPaginated(ginboot.PageRequest{
		Page: 0,
		Size: 10,
		Sort: ginboot.SortField{Field: "unknown", Direction: 1},
	})
	if len(pageRes.Contents) != 3 {
		t.Fatalf("expected items even if sort field missing")
	}

	// Out of bounds page
	pageRes, _ = repo.FindAllPaginated(ginboot.PageRequest{
		Page: 5, // way out of bounds
		Size: 2,
	})
	if len(pageRes.Contents) != 0 {
		t.Fatalf("expected empty contents for out of bounds page")
	}

	// Negative page bounds (should default to 0)
	pageRes, _ = repo.FindByPaginated(ginboot.PageRequest{
		Page: -1,
		Size: 2,
	}, map[string]interface{}{})
	if len(pageRes.Contents) != 2 {
		t.Fatalf("expected 2 items for negative page (default to 0)")
	}
}
