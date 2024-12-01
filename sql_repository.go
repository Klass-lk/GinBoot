package ginboot

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

type SQLRepository[T Document] struct {
	db        *sql.DB
	tableName string
}

func NewSQLRepository[T Document](db *sql.DB) *SQLRepository[T] {
	var doc T
	return &SQLRepository[T]{
		db:        db,
		tableName: doc.GetCollectionName(),
	}
}

func (r *SQLRepository[T]) FindById(id string) (T, error) {
	var result T
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1", r.tableName)
	row := r.db.QueryRow(query, id)
	err := row.Scan(&result)
	return result, err
}

func (r *SQLRepository[T]) FindAllById(ids []string) ([]T, error) {
	var results []T
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE id IN (%s)",
		r.tableName, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var result T
		if err := rows.Scan(&result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func (r *SQLRepository[T]) Save(doc T) error {
	fields, values := r.extractFieldsAndValues(doc)
	placeholders := make([]string, len(values))
	for i := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		r.tableName,
		strings.Join(fields, ","),
		strings.Join(placeholders, ","))

	_, err := r.db.Exec(query, values...)
	return err
}

func (r *SQLRepository[T]) SaveOrUpdate(doc T) error {
	fields, values := r.extractFieldsAndValues(doc)
	placeholders := make([]string, len(values))
	updates := make([]string, len(fields))

	for i := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		updates[i] = fmt.Sprintf("%s = $%d", fields[i], i+1)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (id) DO UPDATE SET %s",
		r.tableName,
		strings.Join(fields, ","),
		strings.Join(placeholders, ","),
		strings.Join(updates, ","))

	_, err := r.db.Exec(query, values...)
	return err
}

func (r *SQLRepository[T]) SaveAll(docs []T) error {
	if len(docs) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}

	for _, doc := range docs {
		if err := r.Save(doc); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (r *SQLRepository[T]) Update(doc T) error {
	fields, values := r.extractFieldsAndValues(doc)
	updates := make([]string, len(fields))

	for i := range fields {
		updates[i] = fmt.Sprintf("%s = $%d", fields[i], i+1)
	}

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d",
		r.tableName,
		strings.Join(updates, ","),
		len(values))

	_, err := r.db.Exec(query, values...)
	return err
}

func (r *SQLRepository[T]) Delete(id string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", r.tableName)
	_, err := r.db.Exec(query, id)
	return err
}

func (r *SQLRepository[T]) FindOneBy(field string, value interface{}) (T, error) {
	var result T
	query := fmt.Sprintf("SELECT * FROM %s WHERE %s = $1", r.tableName, field)
	row := r.db.QueryRow(query, value)
	err := row.Scan(&result)
	return result, err
}

func (r *SQLRepository[T]) FindOneByFilters(filters map[string]interface{}) (T, error) {
	var result T
	conditions, values := r.buildWhereClause(filters)
	query := fmt.Sprintf("SELECT * FROM %s WHERE %s", r.tableName, conditions)
	row := r.db.QueryRow(query, values...)
	err := row.Scan(&result)
	return result, err
}

func (r *SQLRepository[T]) FindBy(field string, value interface{}) ([]T, error) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE %s = $1", r.tableName, field)
	rows, err := r.db.Query(query, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []T
	for rows.Next() {
		var result T
		if err := rows.Scan(&result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *SQLRepository[T]) FindByFilters(filters map[string]interface{}) ([]T, error) {
	conditions, values := r.buildWhereClause(filters)
	query := fmt.Sprintf("SELECT * FROM %s WHERE %s", r.tableName, conditions)
	rows, err := r.db.Query(query, values...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []T
	for rows.Next() {
		var result T
		if err := rows.Scan(&result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *SQLRepository[T]) FindAll(options ...interface{}) ([]T, error) {
	query := fmt.Sprintf("SELECT * FROM %s", r.tableName)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []T
	for rows.Next() {
		var result T
		if err := rows.Scan(&result); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *SQLRepository[T]) FindAllPaginated(pageRequest PageRequest) (PageResponse[T], error) {
	offset := (pageRequest.Page - 1) * pageRequest.Size
	query := fmt.Sprintf("SELECT * FROM %s LIMIT $1 OFFSET $2", r.tableName)

	rows, err := r.db.Query(query, pageRequest.Size, offset)
	if err != nil {
		return PageResponse[T]{}, err
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		var result T
		if err := rows.Scan(&result); err != nil {
			return PageResponse[T]{}, err
		}
		results = append(results, result)
	}
	if err = rows.Err(); err != nil {
		return PageResponse[T]{}, err
	}

	var total int
	err = r.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", r.tableName)).Scan(&total)
	if err != nil {
		return PageResponse[T]{}, err
	}

	return PageResponse[T]{
		Contents:         results,
		NumberOfElements: pageRequest.Size,
		Pageable:         pageRequest,
		TotalElements:    total,
		TotalPages:       (total + pageRequest.Size - 1) / pageRequest.Size,
	}, nil
}

func (r *SQLRepository[T]) FindByPaginated(pageRequest PageRequest, filters map[string]interface{}) (PageResponse[T], error) {
	conditions, values := r.buildWhereClause(filters)
	offset := (pageRequest.Page - 1) * pageRequest.Size

	query := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT $%d OFFSET $%d",
		r.tableName, conditions, len(values)+1, len(values)+2)

	queryValues := append(values, pageRequest.Size, offset)
	rows, err := r.db.Query(query, queryValues...)
	if err != nil {
		return PageResponse[T]{}, err
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		var result T
		if err := rows.Scan(&result); err != nil {
			return PageResponse[T]{}, err
		}
		results = append(results, result)
	}
	if err = rows.Err(); err != nil {
		return PageResponse[T]{}, err
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", r.tableName, conditions)
	var total int
	err = r.db.QueryRow(countQuery, values...).Scan(&total)
	if err != nil {
		return PageResponse[T]{}, err
	}

	return PageResponse[T]{
		Contents:         results,
		NumberOfElements: pageRequest.Size,
		Pageable:         pageRequest,
		TotalElements:    total,
		TotalPages:       (total + pageRequest.Size - 1) / pageRequest.Size,
	}, nil
}

func (r *SQLRepository[T]) CountBy(field string, value interface{}) (int64, error) {
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", r.tableName, field)
	err := r.db.QueryRow(query, value).Scan(&count)
	return count, err
}

func (r *SQLRepository[T]) CountByFilters(filters map[string]interface{}) (int64, error) {
	conditions, values := r.buildWhereClause(filters)
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", r.tableName, conditions)
	err := r.db.QueryRow(query, values...).Scan(&count)
	return count, err
}

func (r *SQLRepository[T]) ExistsBy(field string, value interface{}) (bool, error) {
	count, err := r.CountBy(field, value)
	return count > 0, err
}

func (r *SQLRepository[T]) ExistsByFilters(filters map[string]interface{}) (bool, error) {
	count, err := r.CountByFilters(filters)
	return count > 0, err
}

func (r *SQLRepository[T]) scanRow(row *sql.Row, dest *T) error {
	var item T
	fields, _ := r.extractFieldsAndValues(item)
	values := make([]interface{}, len(fields))
	scanValues := make([]interface{}, len(fields))
	for i := range values {
		scanValues[i] = &values[i]
	}
	if err := row.Scan(scanValues...); err != nil {
		return err
	}
	// TODO: Map values back to dest
	return nil
}

func (r *SQLRepository[T]) scanSingleRow(rows *sql.Rows, dest *T) error {
	fields, _ := r.extractFieldsAndValues(*dest)
	values := make([]interface{}, len(fields))
	scanValues := make([]interface{}, len(fields))
	for i := range values {
		scanValues[i] = &values[i]
	}
	if err := rows.Scan(scanValues...); err != nil {
		return err
	}
	// TODO: Map values back to dest
	return nil
}

func (r *SQLRepository[T]) scanRows(rows *sql.Rows) ([]T, error) {
	var results []T
	for rows.Next() {
		var item T
		if err := r.scanSingleRow(rows, &item); err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

func (r *SQLRepository[T]) extractFieldsAndValues(doc T) ([]string, []interface{}) {
	v := reflect.ValueOf(doc)
	t := v.Type()
	var fields []string
	var values []interface{}

	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		fields = append(fields, tag)
		values = append(values, v.Field(i).Interface())
	}
	return fields, values
}

func (r *SQLRepository[T]) buildWhereClause(filters map[string]interface{}) (string, []interface{}) {
	var conditions []string
	var values []interface{}
	i := 1

	for field, value := range filters {
		conditions = append(conditions, fmt.Sprintf("%s = $%d", field, i))
		values = append(values, value)
		i++
	}

	return strings.Join(conditions, " AND "), values
}
