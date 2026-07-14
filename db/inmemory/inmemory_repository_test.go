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
