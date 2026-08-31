package middleware

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCompressEngine(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Compress())
	r.GET("/", handler)
	return r
}

func do(t *testing.T, engine *gin.Engine, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// A list endpoint's JSON is the case this middleware exists for: repetitive,
// well over the threshold, and served to clients that advertise gzip.
func TestCompressGzipsLargeJSON(t *testing.T) {
	payload := make([]map[string]string, 60)
	for i := range payload {
		payload[i] = map[string]string{"content": "a bullet journal entry", "status": "open", "logKind": "weekly"}
	}

	engine := newCompressEngine(func(c *gin.Context) { c.JSON(http.StatusOK, payload) })
	rec := do(t, engine, "gzip")

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
		t.Errorf("Vary = %q, want it to include Accept-Encoding", got)
	}

	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("response is not valid gzip: %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading gzip body: %v", err)
	}

	var round []map[string]string
	if err := json.Unmarshal(decoded, &round); err != nil {
		t.Fatalf("decompressed body is not the original JSON: %v", err)
	}
	if len(round) != len(payload) {
		t.Errorf("decoded %d entries, want %d", len(round), len(payload))
	}
	if rec.Body.Len() >= len(decoded) {
		t.Errorf("compressed size %d is not smaller than raw size %d", rec.Body.Len(), len(decoded))
	}
}

// Below the threshold a gzip frame costs more than it saves, and the body must
// still arrive verbatim with an accurate length.
func TestCompressSkipsSmallResponses(t *testing.T) {
	engine := newCompressEngine(func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	rec := do(t, engine, "gzip")

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want it unset for a small body", got)
	}
	body := rec.Body.String()
	if body != `{"status":"ok"}` {
		t.Errorf("body = %q, want the original JSON", body)
	}
	if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Errorf("Content-Length = %q, want %d", got, len(body))
	}
}

// A client that did not ask for gzip must get the bytes unchanged.
func TestCompressLeavesNonAcceptingClientsAlone(t *testing.T) {
	payload := strings.Repeat("entry ", 400)
	engine := newCompressEngine(func(c *gin.Context) { c.String(http.StatusOK, payload) })
	rec := do(t, engine, "")

	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want it unset", got)
	}
	if rec.Body.String() != payload {
		t.Error("body was altered for a client that did not accept gzip")
	}
}

// Status codes must survive the buffering, including ones that carry no body.
func TestCompressPreservesStatusCodes(t *testing.T) {
	t.Run("error with body", func(t *testing.T) {
		engine := newCompressEngine(func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		})
		rec := do(t, engine, "gzip")

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if rec.Body.String() != `{"error":"authentication required"}` {
			t.Errorf("body = %q", rec.Body.String())
		}
	})

	t.Run("no content", func(t *testing.T) {
		engine := newCompressEngine(func(c *gin.Context) { c.Status(http.StatusNoContent) })
		rec := do(t, engine, "gzip")

		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want empty", rec.Body.String())
		}
	})
}

func TestAcceptsGzip(t *testing.T) {
	cases := map[string]bool{
		"":                             false,
		"gzip":                         true,
		"GZIP":                         true,
		"deflate, gzip;q=1.0, *;q=0.5": true,
		"br":                           false,
		"deflate":                      false,
	}
	for header, want := range cases {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}
