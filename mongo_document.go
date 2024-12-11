package ginboot

import (
	"reflect"
)

type Document interface {
	GetCollectionName() string
}

// getDocumentID returns the ID value of a document using reflection
// It looks for a field tagged with `ginboot:"_id"` or falls back to a field named "ID"
func getDocumentID(doc interface{}) string {
	val := reflect.ValueOf(doc)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if tag := field.Tag.Get("ginboot"); tag == "_id" {
			return val.Field(i).String()
		}
	}

	// Fallback to ID field if no tag is found
	if idField := val.FieldByName("Id"); idField.IsValid() {
		return idField.String()
	}

	return ""
}
