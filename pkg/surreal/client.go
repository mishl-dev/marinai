package surreal

import (
	"context"
	"fmt"
	"reflect"
	"regexp"

	"github.com/surrealdb/surrealdb.go"
)

type Client struct {
	db *surrealdb.DB
}

// identifierRegex ensures that table names and fields only contain alphanumeric characters and underscores
var identifierRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func validateIdentifier(s string) error {
	if !identifierRegex.MatchString(s) {
		return fmt.Errorf("invalid identifier: %s", s)
	}
	return nil
}

func NewClient(host, user, pass, namespace, database string) (*Client, error) {
	db, err := surrealdb.New(host)
	if err != nil {
		return nil, fmt.Errorf("failed to create surrealdb client: %w", err)
	}

	if _, err = db.SignIn(context.Background(), map[string]interface{}{
		"user": user,
		"pass": pass,
	}); err != nil {
		return nil, fmt.Errorf("failed to signin to surrealdb: %w", err)
	}

	if err = db.Use(context.Background(), namespace, database); err != nil {
		return nil, fmt.Errorf("failed to use surrealdb namespace/database: %w", err)
	}

	return &Client{db: db}, nil
}

func (c *Client) Close() {
	c.db.Close(context.Background())
}

func (c *Client) Query(sql string, vars interface{}) (interface{}, error) {
	result, err := surrealdb.Query[interface{}](context.Background(), c.db, sql, vars.(map[string]interface{}))
	if err != nil {
		return nil, err
	}

	// Unwrap the result: *RawQueryResponse -> Result field
	rv := reflect.ValueOf(result)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Struct {
		resField := rv.FieldByName("Result")
		if resField.IsValid() {
			return resField.Interface(), nil
		}
	} else if rv.Kind() == reflect.Slice {
		// Handle slice of results (e.g. []QueryResult)
		if rv.Len() > 0 {
			// Return the result of the last query (or the only one)
			lastElem := rv.Index(rv.Len() - 1)
			if lastElem.Kind() == reflect.Struct {
				resField := lastElem.FieldByName("Result")
				if resField.IsValid() {
					return resField.Interface(), nil
				}
			}
		}
	}

	return result, nil
}

func (c *Client) Create(thing string, data interface{}) (interface{}, error) {
	result, err := surrealdb.Create[interface{}](context.Background(), c.db, thing, data)
	if err != nil {
		return nil, err
	}
	return result, nil
}
func (c *Client) Select(thing string) (interface{}, error) {
	result, err := surrealdb.Select[interface{}](context.Background(), c.db, thing)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// VectorSearch performs a cosine similarity search
func (c *Client) VectorSearch(table string, vectorField string, queryVector []float32, limit int, filter map[string]interface{}) ([]interface{}, error) {
	// Validate inputs to prevent SQL injection
	if err := validateIdentifier(table); err != nil {
		return nil, err
	}
	if err := validateIdentifier(vectorField); err != nil {
		return nil, err
	}

	whereClause, err := buildWhereClause(filter)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT *, vector::similarity::cosine(%s, $query_vector) AS similarity 
		FROM %s 
		WHERE %s 
		ORDER BY similarity DESC 
		LIMIT %d;
	`, vectorField, table, whereClause, limit)

	vars := map[string]interface{}{
		"query_vector": queryVector,
	}

	for k, v := range filter {
		vars[k] = v
	}

	result, err := c.Query(query, vars)
	if err != nil {
		return nil, err
	}

	// Result is already unwrapped by Query
	rows, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}

	return rows, nil
}

func buildWhereClause(filter map[string]interface{}) (string, error) {
	if len(filter) == 0 {
		return "true", nil
	}
	clause := ""
	i := 0
	for k := range filter {
		// Validate filter keys
		if err := validateIdentifier(k); err != nil {
			return "", err
		}

		if i > 0 {
			clause += " AND "
		}
		clause += fmt.Sprintf("%s = $%s", k, k)
		i++
	}
	return clause, nil
}
