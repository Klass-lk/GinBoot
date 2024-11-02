package types

type Document interface {
	GetID() string
	GetCollectionName() string
}
