package ginboot

type Document interface {
	GetID() string
	GetCollectionName() string
}
