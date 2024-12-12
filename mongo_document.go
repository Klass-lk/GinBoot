package ginboot

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"unicode"
)

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

// getCollectionNameFromComment extracts collection name from type comment
// Comment format: // ginboot:collection:name
func getCollectionNameFromComment(doc interface{}) string {
	// Get the package path and type name
	typ := reflect.TypeOf(doc)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	_, filename, _, _ := runtime.Caller(1)
	dir := filepath.Dir(filename)

	// Parse the file
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
	if err != nil {
		return ""
	}

	// Look for the type declaration and its doc comment
	typeName := typ.Name()
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							if typeSpec.Name.Name == typeName {
								if genDecl.Doc != nil {
									for _, comment := range genDecl.Doc.List {
										text := comment.Text
										const prefix = "// ginboot:collection:"
										if strings.HasPrefix(text, prefix) {
											return strings.TrimSpace(strings.TrimPrefix(text, prefix))
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return ""
}

// getCollectionNameFromType converts type name to snake_case
// Example: UserProfile -> user_profile
func getCollectionNameFromType(typ reflect.Type) string {
	name := typ.Name()
	var result strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// getCollectionName returns the collection name for a document type
// First tries to get name from comment, then falls back to type name
func getCollectionName(doc interface{}) string {
	typ := reflect.TypeOf(doc)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	// Try to get collection name from comment
	if name := getCollectionNameFromComment(doc); name != "" {
		return name
	}

	// Fall back to type name in snake_case
	return getCollectionNameFromType(typ)
}
