package ginboot

type Document interface {
	GetID() interface{}
	GetCollectionName() string
}
