// Package testutil provides an in-process fake NocoDB Meta API v3 server for
// unit tests. It implements just enough of the table/field/record endpoints to
// drive the api client, storage, and migration executor without a network or a
// real NocoDB instance, while recording every request so tests can assert the
// exact wire contract (method, path, headers, body) the tool sends.
package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/memclutter/nocodb-migrator/internal/api"
)

// TestToken is the API token the fake expects on every request.
const TestToken = "nc_pat_testtoken"

// TestBaseID is the base id the fake is scoped to.
const TestBaseID = "ptest000base"

// RecordedRequest is a single request the fake observed.
type RecordedRequest struct {
	Method string
	Path   string
	Query  map[string][]string
	Token  string
	Body   []byte
}

// DecodeBody unmarshals the recorded body into v.
func (r RecordedRequest) DecodeBody(v interface{}) error {
	return json.Unmarshal(r.Body, v)
}

type fakeField struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type fakeTable struct {
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Desc   string      `json:"description,omitempty"`
	Fields []fakeField `json:"fields,omitempty"`
}

type fakeRecord struct {
	id     int
	fields map[string]interface{}
}

// FakeNocoDB is a stateful in-memory NocoDB Meta API v3 server.
type FakeNocoDB struct {
	Server *httptest.Server

	mu        sync.Mutex
	requests  []RecordedRequest
	tables    map[string]*fakeTable
	records   map[string][]fakeRecord
	tableSeq  int
	fieldSeq  int
	recordSeq int

	// failNextCreateTable, when set, makes the next POST /tables return a
	// NocoDB-style error with this message (used to exercise error paths).
	failNextCreateTable string
}

var (
	reTables      = regexp.MustCompile(`^/api/v3/meta/bases/([^/]+)/tables$`)
	reTableID     = regexp.MustCompile(`^/api/v3/meta/bases/([^/]+)/tables/([^/]+)$`)
	reTableFields = regexp.MustCompile(`^/api/v3/meta/bases/([^/]+)/tables/([^/]+)/fields$`)
	reFieldID     = regexp.MustCompile(`^/api/v3/meta/bases/([^/]+)/fields/([^/]+)$`)
	reRecords     = regexp.MustCompile(`^/api/v3/data/([^/]+)/([^/]+)/records$`)
)

// NewFake starts a fake NocoDB and registers cleanup on the test.
func NewFake(t *testing.T) *FakeNocoDB {
	t.Helper()
	f := &FakeNocoDB{
		tables:  make(map[string]*fakeTable),
		records: make(map[string][]fakeRecord),
	}
	f.Server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.Server.Close)
	return f
}

// Client returns an api client wired to the fake and the test base id.
func (f *FakeNocoDB) Client() *api.Client {
	return api.NewClient(f.Server.URL, TestToken, TestBaseID)
}

// Requests returns a copy of every request the fake has observed.
func (f *FakeNocoDB) Requests() []RecordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RecordedRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// FailNextCreateTable makes the next table creation return an API error.
func (f *FakeNocoDB) FailNextCreateTable(message string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNextCreateTable = message
}

func (f *FakeNocoDB) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	f.mu.Lock()
	f.requests = append(f.requests, RecordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Token:  r.Header.Get("xc-token"),
		Body:   body,
	})
	f.mu.Unlock()

	switch {
	case reTables.MatchString(r.URL.Path):
		f.handleTables(w, r, body)
	case reTableFields.MatchString(r.URL.Path):
		m := reTableFields.FindStringSubmatch(r.URL.Path)
		f.handleTableFields(w, r, m[2], body)
	case reTableID.MatchString(r.URL.Path):
		m := reTableID.FindStringSubmatch(r.URL.Path)
		f.handleTableID(w, r, m[2], body)
	case reFieldID.MatchString(r.URL.Path):
		m := reFieldID.FindStringSubmatch(r.URL.Path)
		f.handleFieldID(w, r, m[2], body)
	case reRecords.MatchString(r.URL.Path):
		m := reRecords.FindStringSubmatch(r.URL.Path)
		f.handleRecords(w, r, m[2], body)
	default:
		writeErr(w, http.StatusNotFound, "unknown endpoint: "+r.URL.Path)
	}
}

func (f *FakeNocoDB) handleTables(w http.ResponseWriter, r *http.Request, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		list := make([]*fakeTable, 0, len(f.tables))
		for _, t := range f.tables {
			list = append(list, t)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"list": list})
	case http.MethodPost:
		if f.failNextCreateTable != "" {
			msg := f.failNextCreateTable
			f.failNextCreateTable = ""
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
		var req api.TableCreate
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid table body")
			return
		}
		f.tableSeq++
		id := fmt.Sprintf("tbl%d", f.tableSeq)
		tbl := &fakeTable{ID: id, Title: req.Title, Desc: req.Description}
		for _, fc := range req.Fields {
			f.fieldSeq++
			tbl.Fields = append(tbl.Fields, fakeField{
				ID:    fmt.Sprintf("fld%d", f.fieldSeq),
				Title: fc.Title,
				Type:  fc.Type,
			})
		}
		f.tables[id] = tbl
		writeJSON(w, http.StatusOK, tbl)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (f *FakeNocoDB) handleTableID(w http.ResponseWriter, r *http.Request, id string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	tbl, ok := f.tables[id]
	if !ok {
		writeErr(w, http.StatusNotFound, "table not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, tbl)
	case http.MethodPatch:
		var req api.TableUpdate
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid table update")
			return
		}
		if req.Title != "" {
			tbl.Title = req.Title
		}
		if req.Description != "" {
			tbl.Desc = req.Description
		}
		writeJSON(w, http.StatusOK, tbl)
	case http.MethodDelete:
		delete(f.tables, id)
		delete(f.records, id)
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (f *FakeNocoDB) handleTableFields(w http.ResponseWriter, r *http.Request, tableID string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tbl, ok := f.tables[tableID]
	if !ok {
		writeErr(w, http.StatusNotFound, "table not found")
		return
	}
	var req api.FieldCreate
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid field body")
		return
	}
	f.fieldSeq++
	fld := fakeField{ID: fmt.Sprintf("fld%d", f.fieldSeq), Title: req.Title, Type: req.Type}
	tbl.Fields = append(tbl.Fields, fld)
	writeJSON(w, http.StatusOK, fld)
}

func (f *FakeNocoDB) handleFieldID(w http.ResponseWriter, r *http.Request, fieldID string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	tbl, fld := f.findField(fieldID)
	if fld == nil {
		writeErr(w, http.StatusNotFound, "field not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, fld)
	case http.MethodPatch:
		var req api.FieldUpdate
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid field update")
			return
		}
		if req.Title != "" {
			fld.Title = req.Title
		}
		if req.Type != "" {
			fld.Type = req.Type
		}
		writeJSON(w, http.StatusOK, fld)
	case http.MethodDelete:
		f.removeField(tbl, fieldID)
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (f *FakeNocoDB) handleRecords(w http.ResponseWriter, r *http.Request, tableID string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		recs := f.records[tableID]
		out := make([]map[string]interface{}, 0, len(recs))
		for _, rec := range recs {
			out = append(out, map[string]interface{}{"id": rec.id, "fields": rec.fields})
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"records": out})
	case http.MethodPost:
		var payload struct {
			Fields map[string]interface{} `json:"fields"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid record body")
			return
		}
		f.recordSeq++
		rec := fakeRecord{id: f.recordSeq, fields: payload.Fields}
		f.records[tableID] = append(f.records[tableID], rec)
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": rec.id, "fields": rec.fields})
	case http.MethodDelete:
		var ids []map[string]interface{}
		if err := json.Unmarshal(body, &ids); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid delete body")
			return
		}
		del := make(map[int]bool)
		for _, m := range ids {
			if n, ok := toInt(m["id"]); ok {
				del[n] = true
			}
		}
		kept := f.records[tableID][:0]
		for _, rec := range f.records[tableID] {
			if !del[rec.id] {
				kept = append(kept, rec)
			}
		}
		f.records[tableID] = kept
		writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (f *FakeNocoDB) findField(fieldID string) (*fakeTable, *fakeField) {
	for _, tbl := range f.tables {
		for i := range tbl.Fields {
			if tbl.Fields[i].ID == fieldID {
				return tbl, &tbl.Fields[i]
			}
		}
	}
	return nil, nil
}

func (f *FakeNocoDB) removeField(tbl *fakeTable, fieldID string) {
	kept := tbl.Fields[:0]
	for _, fld := range tbl.Fields {
		if fld.ID != fieldID {
			kept = append(kept, fld)
		}
	}
	tbl.Fields = kept
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case string:
		var i int
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%d", &i); err == nil {
			return i, true
		}
	}
	return 0, false
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, api.APIError{Message: message, Error: http.StatusText(status)})
}
