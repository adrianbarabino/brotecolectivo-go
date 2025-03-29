package utils

import (
	"bytes"
	"net/http"
)

// ResponseCapture is a wrapper around http.ResponseWriter that captures the response
type ResponseCapture struct {
	http.ResponseWriter
	StatusCode int
	Body       *bytes.Buffer
}

// NewResponseCapture creates a new ResponseCapture
func NewResponseCapture(w http.ResponseWriter) *ResponseCapture {
	return &ResponseCapture{
		ResponseWriter: w,
		Body:           &bytes.Buffer{},
		StatusCode:     http.StatusOK,
	}
}

// WriteHeader captures the status code and passes it to the underlying ResponseWriter
func (r *ResponseCapture) WriteHeader(statusCode int) {
	r.StatusCode = statusCode
	if statusCode >= 400 {
		r.ResponseWriter.WriteHeader(statusCode)
	}
}

// Write captures the response body and returns the number of bytes written
func (r *ResponseCapture) Write(b []byte) (int, error) {
	if r.StatusCode >= 400 {
		return r.ResponseWriter.Write(b)
	}
	return r.Body.Write(b)
}
