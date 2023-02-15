package summer

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com", nil)
	req.Header.Set("X-Forwarded-For", "10.10.10.10, 80.12.23.44")

	require.Equal(t, "80.12.23.44", extractClientIP(req))

	req = httptest.NewRequest("GET", "https://example.com", nil)
	req.RemoteAddr = "80.80.80.80:14443"
	req.Header.Set("X-Forwarded-For", "10.10.10.10, 80.12.23.44")

	require.Equal(t, "80.12.23.44", extractClientIP(req))

	req = httptest.NewRequest("GET", "https://example.com", nil)
	req.RemoteAddr = "80.80.80.80:14443"
	req.Header.Set("X-Forwarded-For", ", ")

	require.Equal(t, "80.80.80.80", extractClientIP(req))
}

func TestRespondText(t *testing.T) {
	rw := httptest.NewRecorder()
	respondText(rw, "OK", http.StatusTeapot)
	require.Equal(t, rw.Code, http.StatusTeapot)
	require.Equal(t, rw.Body.String(), "OK")
}

func TestFlattenSlice(t *testing.T) {
	require.Equal(t, "a", flattenSlice([]string{"a"}))
	require.Equal(t, []int{1, 2}, flattenSlice([]int{1, 2}))
}

func TestFlattenRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com/get?aaa=bbb", nil)

	m := map[string]any{}
	err := flattenRequest(m, req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"aaa": "bbb", "query_aaa": "bbb"}, m)

	req = httptest.NewRequest("POST", "https://example.com/post?aaa=bbb", bytes.NewReader([]byte(`{"hello":"world"}`)))
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	m = map[string]any{}
	err = flattenRequest(m, req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"aaa": "bbb", "header_content_type": "application/json;charset=utf-8", "hello": "world", "query_aaa": "bbb"}, m)

	req = httptest.NewRequest("POST", "https://example.com/post?aaa=bbb", bytes.NewReader([]byte(`hello=world`)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=utf-8")

	m = map[string]any{}
	err = flattenRequest(m, req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"aaa": "bbb", "header_content_type": "application/x-www-form-urlencoded;charset=utf-8", "hello": "world", "query_aaa": "bbb"}, m)

	req = httptest.NewRequest("POST", "https://example.com/post?aaa=bbb", bytes.NewReader([]byte(`hello=world`)))
	req.Header.Set("Content-Type", "text/plain;charset=utf-8")

	m = map[string]any{}
	err = flattenRequest(m, req)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"aaa": "bbb", "header_content_type": "text/plain;charset=utf-8", "query_aaa": "bbb", "text": "hello=world"}, m)

	req = httptest.NewRequest("POST", "https://example.com/post?aaa=bbb", bytes.NewReader([]byte(`hello=world`)))
	req.Header.Set("Content-Type", "application/x-custom")

	m = map[string]any{}
	err = flattenRequest(m, req)
	require.Error(t, err)
}
