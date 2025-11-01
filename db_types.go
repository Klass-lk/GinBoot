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

type TestEntity struct {
	ID        string `ginboot:"id"`
	Name      string
	Value     int
	CreatedAt int64
}

type Document interface {
	GetTableName() string
}
