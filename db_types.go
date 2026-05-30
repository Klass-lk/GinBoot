package ginboot

type SortField struct {
	Field     string `json:"field"`
	Direction int    `json:"direction"`
}

type PageRequest struct {
	Page int       `json:"page"`
	Size int       `json:"size"`
	Sort SortField `json:"sort"`
}

type PageResponse[T interface{}] struct {
	Contents         []T         `json:"content"`
	NumberOfElements int         `json:"numberOfElements"`
	Pageable         PageRequest `json:"pageable"`
	TotalPages       int         `json:"totalPages"`
	TotalElements    int         `json:"totalElements"`
}

type CursorPageRequest struct {
	Size      int       `json:"size"`
	NextToken string    `json:"nextToken"` // Base64 encoded JSON cursor token
	Sort      SortField `json:"sort"`
}

type CursorPageResponse[T any] struct {
	Contents  []T               `json:"content"`
	NextToken string            `json:"nextToken"` // Empty if there are no more pages
	Pageable  CursorPageRequest `json:"pageable"`
}

type TestEntity struct {
	ID    string `ginboot:"id"`
	Name  string
	Value int
}

type Document interface {
	GetTableName() string
}
