package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ http.Flusher = (*responseWriter)(nil)

type countingFlusherRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func newCountingFlusherRecorder() *countingFlusherRecorder {
	return &countingFlusherRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (r *countingFlusherRecorder) Flush() {
	r.flushCount++
}

func TestResponseWriterWriteHeaderOnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)
	rw.WriteHeader(http.StatusInternalServerError)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, http.StatusCreated, rw.statusCode)
}

func TestResponseWriterWriteSetsImplicitStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	n, err := rw.Write([]byte("ok"))

	assert.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, http.StatusOK, rw.statusCode)
	assert.Equal(t, 2, rw.bytesWritten)
}

func TestResponseWriterFlushDelegates(t *testing.T) {
	rec := newCountingFlusherRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.Flush()

	assert.Equal(t, 1, rec.flushCount)
}
