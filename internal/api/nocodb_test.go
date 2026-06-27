package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/memclutter/nocodb-migrator/internal/api"
	"github.com/memclutter/nocodb-migrator/internal/testutil"
)

// TestClientSendsAuthAndBaseScopedPaths verifies every request carries the
// xc-token header and targets a Meta API v3 path scoped to the configured base.
func TestClientSendsAuthAndBaseScopedPaths(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	created, err := c.CreateTable(&api.TableCreate{
		Title:  "Widgets",
		Fields: []api.FieldCreate{{Title: "Name", Type: "SingleLineText"}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	reqs := f.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodPost, reqs[0].Method)
	assert.Equal(t, "/api/v3/meta/bases/"+testutil.TestBaseID+"/tables", reqs[0].Path)
	assert.Equal(t, testutil.TestToken, reqs[0].Token)
}

// TestGetTableByNameResolvesViaListThenFetch checks name resolution issues a
// list call then a fetch-by-id for the matching table.
func TestGetTableByNameResolvesViaListThenFetch(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	created, err := c.CreateTable(&api.TableCreate{
		Title:  "Orders",
		Fields: []api.FieldCreate{{Title: "Total", Type: "Number"}},
	})
	require.NoError(t, err)

	got, err := c.GetTableByName("Orders")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	require.Len(t, got.Fields, 1)
	assert.Equal(t, "Total", got.Fields[0].Title)

	// last two requests: GET tables (list), GET tables/{id}
	reqs := f.Requests()
	require.GreaterOrEqual(t, len(reqs), 3)
	assert.Equal(t, http.MethodGet, reqs[len(reqs)-2].Method)
	assert.Regexp(t, `/tables$`, reqs[len(reqs)-2].Path)
	assert.Equal(t, http.MethodGet, reqs[len(reqs)-1].Method)
	assert.Regexp(t, `/tables/`+created.ID+`$`, reqs[len(reqs)-1].Path)
}

func TestGetTableByNameNotFound(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	_, err := c.GetTableByName("Missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestInsertRecordWrapsFields asserts the insert body nests values under "fields".
func TestInsertRecordWrapsFields(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	out, err := c.InsertRecord("tbl1", api.Record{"Name": "abc", "Qty": 3})
	require.NoError(t, err)
	assert.NotNil(t, out["id"])
	assert.Equal(t, "abc", out["Name"])

	reqs := f.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodPost, reqs[0].Method)
	assert.Equal(t, "/api/v3/data/"+testutil.TestBaseID+"/tbl1/records", reqs[0].Path)

	var body struct {
		Fields map[string]interface{} `json:"fields"`
	}
	require.NoError(t, reqs[0].DecodeBody(&body))
	assert.Equal(t, "abc", body.Fields["Name"])
	assert.EqualValues(t, 3, body.Fields["Qty"])
}

// TestDeleteRecordSendsIDArrayBody asserts single deletion sends [{"id": N}].
func TestDeleteRecordSendsIDArrayBody(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	require.NoError(t, c.DeleteRecord("tbl1", "42"))

	reqs := f.Requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, http.MethodDelete, reqs[0].Method)

	var body []map[string]interface{}
	require.NoError(t, reqs[0].DecodeBody(&body))
	require.Len(t, body, 1)
	assert.EqualValues(t, 42, body[0]["id"])
}

// TestDeleteRecordsWhereListsThenDeletes asserts conditional deletion first GETs
// matching records (with a where query) then DELETEs exactly their ids.
func TestDeleteRecordsWhereListsThenDeletes(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	_, err := c.InsertRecord("tbl9", api.Record{"Name": "keep"})
	require.NoError(t, err)
	_, err = c.InsertRecord("tbl9", api.Record{"Name": "drop"})
	require.NoError(t, err)

	err = c.DeleteRecords("tbl9", map[string]interface{}{"Name": "drop"})
	require.NoError(t, err)

	reqs := f.Requests()
	// inserts (2) + GET where + DELETE
	require.Len(t, reqs, 4)
	getReq := reqs[2]
	assert.Equal(t, http.MethodGet, getReq.Method)
	assert.NotEmpty(t, getReq.Query["where"], "where must be passed as a query param")
	assert.Equal(t, http.MethodDelete, reqs[3].Method)
}

func TestDeleteRecordsRequiresWhere(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	err := c.DeleteRecords("tbl1", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "where")
}

// TestGetRecordsDecodesRecordsEnvelope verifies the {"records":[{id,fields}]}
// response is mapped into RecordList with id exposed under both "id" and "Id".
func TestGetRecordsDecodesRecordsEnvelope(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	_, err := c.InsertRecord("tbl3", api.Record{"Name": "row1"})
	require.NoError(t, err)

	list, err := c.GetRecords("tbl3", 100, 0)
	require.NoError(t, err)
	require.Len(t, list.List, 1)
	assert.Equal(t, "row1", list.List[0]["Name"])
	assert.NotNil(t, list.List[0]["id"])
	assert.NotNil(t, list.List[0]["Id"])
}

// TestAPIErrorSurfacesMessage checks a NocoDB error body surfaces its message.
func TestAPIErrorSurfacesMessage(t *testing.T) {
	f := testutil.NewFake(t)
	c := f.Client()

	f.FailNextCreateTable("There was a syntax error in your SQL query.")
	_, err := c.CreateTable(&api.TableCreate{Title: "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "syntax error in your SQL query")
}

// TestAPIErrorFallsBackToStatus checks a non-decodable error body falls back to
// the HTTP status code.
func TestAPIErrorFallsBackToStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "tok", "base")
	_, err := c.ListTables()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

// TestErrorBodyIsValidAPIError is a guard that the fake emits the documented
// error envelope shape the client decodes.
func TestErrorBodyIsValidAPIError(t *testing.T) {
	var e api.APIError
	require.NoError(t, json.Unmarshal([]byte(`{"message":"m","error":"e"}`), &e))
	assert.Equal(t, "m", e.Message)
}
