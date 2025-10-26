package ginboot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
)

type DBSeeder interface {
	Seed(document string, data *godog.Table) error
}

type TestSuite struct {
	T           *testing.T
	Router      *gin.Engine
	Server      *Server
	Resp        *http.Response
	RespBody    []byte
	Feature     *godog.Feature
	Storage     map[string]string
	RequestBody []byte
	BaseURL     string
	DbSeeders   map[string]DBSeeder
}

type TestLogger struct {
	T *testing.T
}

func (ts *TestSuite) RegisterDBSeeder(document string, seeder DBSeeder) {
	ts.DbSeeders[document] = seeder
}

func (ts *TestSuite) SetBaseURL(baseURL string) {
	ts.BaseURL = baseURL
}

func (ts *TestSuite) InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		ts.Storage = make(map[string]string)
	})
}

func (ts *TestSuite) InitializeScenario(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(sc *godog.Scenario) {
		ts.Resp = nil
		ts.RespBody = nil
		ts.RequestBody = nil
	})

	ctx.Step(`^document "([^"]*)" has the following items$`, ts.documentHasTheFollowingItems)
	ctx.Step(`^I send a POST request to "([^"]*)" with body$`, ts.iSendAPOSTRequestToWithBody)
	ctx.Step(`^I send a GET request to "([^"]*)"$`, ts.iSendAGETRequestTo)
	ctx.Step(`^the response status should be (\d+)$`, ts.theResponseStatusShouldBe)
	ctx.Step(`^the response "([^"]*)" field is stored as "([^"]*)"$`, ts.theResponseFieldIsStoredAs)
	ctx.Step(`^I send an authenticated GET request to "([^"]*)"$`, ts.iSendAnAuthenticatedGETRequestTo)
	ctx.Step(`^the response should contain an item with$`, ts.theResponseShouldContainAnItemWith)
}

func (ts *TestSuite) documentHasTheFollowingItems(document string, data *godog.Table) error {
	seeder, ok := ts.DbSeeders[document]
	if !ok {
		return fmt.Errorf("no seeder registered for document %s", document)
	}
	return seeder.Seed(document, data)
}

func (ts *TestSuite) parseDataTableToJSONs(body *godog.Table) ([]byte, error) {
	if len(body.Rows) < 2 {
		return nil, fmt.Errorf("table must have at least two rows")
	}
	headers := body.Rows[0].Cells
	var data []map[string]interface{}
	for i := 1; i < len(body.Rows); i++ {
		row := body.Rows[i]
		rowData := make(map[string]interface{})
		for j, cell := range row.Cells {
			rowData[headers[j].Value] = cell.Value
		}
		data = append(data, rowData)
	}
	return json.Marshal(data)
}

func (ts *TestSuite) iSendAPOSTRequestToWithBody(path string, body *godog.Table) error {
	var err error
	ts.RequestBody, err = ts.parseDataTableToJSON(body)
	if err != nil {
		return err
	}

	var req *http.Request
	if ts.BaseURL != "" {
		req, err = http.NewRequest("POST", ts.BaseURL+path, bytes.NewBuffer(ts.RequestBody))
	} else {
		req, err = http.NewRequest("POST", path, bytes.NewBuffer(ts.RequestBody))
	}

	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if ts.BaseURL != "" {
		client := &http.Client{}
		ts.Resp, err = client.Do(req)
	} else {
		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)
		ts.Resp = w.Result()
	}

	if err != nil {
		return err
	}
	ts.RespBody, err = ioutil.ReadAll(ts.Resp.Body)
	return err
}

func (ts *TestSuite) iSendAGETRequestTo(path string) error {
	var req *http.Request
	var err error
	if ts.BaseURL != "" {
		req, err = http.NewRequest("GET", ts.BaseURL+path, nil)
	} else {
		req, err = http.NewRequest("GET", path, nil)
	}

	if err != nil {
		return err
	}

	if ts.BaseURL != "" {
		client := &http.Client{}
		ts.Resp, err = client.Do(req)
	} else {
		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)
		ts.Resp = w.Result()
	}

	if err != nil {
		return err
	}
	ts.RespBody, err = ioutil.ReadAll(ts.Resp.Body)
	return err
}

func (ts *TestSuite) iSendAnAuthenticatedGETRequestTo(path string) error {
	var req *http.Request
	var err error
	if ts.BaseURL != "" {
		req, err = http.NewRequest("GET", ts.BaseURL+path, nil)
	} else {
		req, err = http.NewRequest("GET", path, nil)
	}

	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+ts.Storage["authToken"])

	if ts.BaseURL != "" {
		client := &http.Client{}
		ts.Resp, err = client.Do(req)
	} else {
		w := httptest.NewRecorder()
		ts.Router.ServeHTTP(w, req)
		ts.Resp = w.Result()
	}

	if err != nil {
		return err
	}
	ts.RespBody, err = ioutil.ReadAll(ts.Resp.Body)
	return err
}

func (ts *TestSuite) theResponseStatusShouldBe(status int) error {
	assert.Equal(ts.T, status, ts.Resp.StatusCode)
	return nil
}

func (ts *TestSuite) theResponseFieldIsStoredAs(field, key string) error {
	var data map[string]interface{}
	if err := json.Unmarshal(ts.RespBody, &data); err != nil {
		return err
	}
	if val, ok := data[field]; ok {
		ts.Storage[key] = fmt.Sprintf("%v", val)
		return nil
	}
	return fmt.Errorf("field %s not found in response", field)
}

func (ts *TestSuite) theResponseShouldContainAnItemWith(body *godog.Table) error {
	expected, err := ts.parseDataTableToJSON(body)
	if err != nil {
		return err
	}

	var expectedMap map[string]interface{}
	if err := json.Unmarshal(expected, &expectedMap); err != nil {
		return err
	}

	var actualMap map[string]interface{}
	if err := json.Unmarshal(ts.RespBody, &actualMap); err != nil {
		return err
	}

	for key, expectedValue := range expectedMap {
		if actualValue, ok := actualMap[key]; ok {
			assert.Equal(ts.T, expectedValue, actualValue)
		} else {
			return fmt.Errorf("field %s not found in response", key)
		}
	}
	return nil
}

func (ts *TestSuite) parseDataTableToJSON(body *godog.Table) ([]byte, error) {
	if len(body.Rows) < 2 {
		return nil, fmt.Errorf("table must have at least two rows")
	}
	headers := body.Rows[0].Cells
	data := make(map[string]interface{})
	row := body.Rows[1]
	for j, cell := range row.Cells {
		data[headers[j].Value] = cell.Value
	}
	return json.Marshal(data)
}

// GenericDBSeeder is a sample DBSeeder that uses reflection to populate structs.
// Users can use this as a starting point for their own seeders.
type GenericDBSeeder struct {
	Constructors map[string]func() interface{}
	DB           *mongo.Database
}

func NewGenericDBSeeder(db *mongo.Database) *GenericDBSeeder {
	return &GenericDBSeeder{
		Constructors: make(map[string]func() interface{}),
		DB:           db,
	}
}

func (gds *GenericDBSeeder) Register(name string, constructor func() interface{}) {
	gds.Constructors[name] = constructor
}

func (gds *GenericDBSeeder) Seed(document string, data *godog.Table) error {
	constructor, ok := gds.Constructors[document]
	if !ok {
		return fmt.Errorf("no constructor registered for document type: %s", document)
	}

	headers := data.Rows[0].Cells
	for i := 1; i < len(data.Rows); i++ {
		row := data.Rows[i]
		docInstance := constructor() // Create a new instance of the document struct

		val := reflect.ValueOf(docInstance).Elem()
		typ := val.Type()

		for j, cell := range row.Cells {
			fieldName := headers[j].Value
			goFieldName := toPascalCase(fieldName)

			field := val.FieldByName(goFieldName)
			if !field.IsValid() {
				for k := 0; k < typ.NumField(); k++ {
					structField := typ.Field(k)
					if jsonTag := structField.Tag.Get("json"); jsonTag == fieldName {
						field = val.Field(k)
						break
					}
				}
			}

			if field.IsValid() && field.CanSet() {
				switch field.Kind() {
				case reflect.String:
					field.SetString(cell.Value)
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					if cell.Value == "" {
						field.SetInt(0)
					} else {
						intVal, err := strconv.Atoi(cell.Value)
						if err != nil {
							return fmt.Errorf("failed to parse int for field %s: %w", fieldName, err)
						}
						field.SetInt(int64(intVal))
					}
				case reflect.Bool:
					if cell.Value == "" {
						field.SetBool(false)
					} else {
						boolVal, err := strconv.ParseBool(cell.Value)
						if err != nil {
							return fmt.Errorf("failed to parse bool for field %s: %w", fieldName, err)
						}
						field.SetBool(boolVal)
					}
				default:
					return fmt.Errorf("unsupported field type for %s: %s", fieldName, field.Kind())
				}
			} else {
				return fmt.Errorf("could not set field %s for document %s", fieldName, document)
			}
		}
		// Now 'docInstance' is populated. You would typically insert it into your database.
		_, err := gds.DB.Collection(document).InsertOne(context.Background(), docInstance)
		if err != nil {
			return err
		}
	}
	return nil
}

func toPascalCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (tl *TestLogger) Write(p []byte) (n int, err error) {
	if tl.T != nil {
		tl.T.Logf("%s", p)
	}
	return len(p), nil
}

func TestFeatures(t *testing.T, suite *TestSuite) {
	suite.T = t
	opts := godog.Options{
		Format:    "pretty",
		Output:    colors.Colored(&TestLogger{T: t}),
		Paths:     []string{"features"},
		Strict:    true,
		Randomize: 0,
	}

	godog.TestSuite{
		Name:                 "ginboot",
		TestSuiteInitializer: suite.InitializeTestSuite,
		ScenarioInitializer:  suite.InitializeScenario,
		Options:              &opts,
	}.Run()
}
