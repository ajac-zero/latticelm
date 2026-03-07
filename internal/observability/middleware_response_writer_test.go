package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ http.Flusher = (*metricsResponseWriter)(nil)
var _ http.Flusher = (*statusResponseWriter)(nil)

type testFlusherRecorder struct {
	*httptest.ResponseRecorder
	flushCount int
}

func newTestFlusherRecorder() *testFlusherRecorder {
	return &testFlusherRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (r *testFlusherRecorder) Flush() {
	r.flushCount++
}

func TestMetricsResponseWriterWriteHeaderOnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusAccepted)
	rw.WriteHeader(http.StatusInternalServerError)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, http.StatusAccepted, rw.statusCode)
}

func TestMetricsResponseWriterFlushDelegates(t *testing.T) {
	rec := newTestFlusherRecorder()
	rw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.Flush()

	assert.Equal(t, 1, rec.flushCount)
}

func TestStatusResponseWriterWriteHeaderOnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &statusResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNoContent)
	rw.WriteHeader(http.StatusInternalServerError)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, http.StatusNoContent, rw.statusCode)
}

func TestStatusResponseWriterFlushDelegates(t *testing.T) {
	rec := newTestFlusherRecorder()
	rw := &statusResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.Flush()

	assert.Equal(t, 1, rec.flushCount)
}
