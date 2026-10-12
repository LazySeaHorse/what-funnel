package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

func TestPathUUID(t *testing.T) {
	var got uuid.UUID
	var ok bool
	r := mux.NewRouter()
	r.HandleFunc("/x/{id}", func(w http.ResponseWriter, req *http.Request) {
		got, ok = pathUUID(w, req, "id", "invalid thing ID")
	})

	id := uuid.New()
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x/"+id.String(), nil))
	assert.True(t, ok)
	assert.Equal(t, id, got)

	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x/nope", nil))
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.JSONEq(t, `{"error":"invalid thing ID"}`, rr.Body.String())
}

func TestDecodeJSON(t *testing.T) {
	var dst struct{ A string }
	rr := httptest.NewRecorder()
	assert.True(t, decodeJSON(rr, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"A":"b"}`)), &dst))
	assert.Equal(t, "b", dst.A)

	rr = httptest.NewRecorder()
	assert.False(t, decodeJSON(rr, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`)), &dst))
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.JSONEq(t, `{"error":"invalid body"}`, rr.Body.String())
}
