package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type nonFlusherRecorder struct {
	recorder         *httptest.ResponseRecorder
	writeHeaderCalls int
}

func newNonFlusherRecorder() *nonFlusherRecorder {
	return &nonFlusherRecorder{recorder: httptest.NewRecorder()}
}

func (w *nonFlusherRecorder) Header() http.Header {
	return w.recorder.Header()
}

func (w *nonFlusherRecorder) Write(b []byte) (int, error) {
	return w.recorder.Write(b)
}

func (w *nonFlusherRecorder) WriteHeader(statusCode int) {
	w.writeHeaderCalls++
	w.recorder.WriteHeader(statusCode)
}

func (w *nonFlusherRecorder) StatusCode() int {
	return w.recorder.Code
}

func (w *nonFlusherRecorder) BodyString() string {
	return w.recorder.Body.String()
}

func TestHandleStreamingResponseWithoutFlusherWritesSingleErrorHeader(t *testing.T) {
	s := New(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	w := newNonFlusherRecorder()

	s.handleStreamingResponse(w, req, nil, nil, nil, nil, nil)

	assert.Equal(t, 1, w.writeHeaderCalls)
	assert.Equal(t, http.StatusInternalServerError, w.StatusCode())
	assert.Contains(t, w.BodyString(), "streaming not supported")
}
