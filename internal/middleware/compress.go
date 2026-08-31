package middleware

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// minCompressBytes is the response size below which compressing costs more
// than it saves — a gzip frame has ~20 bytes of overhead, and tiny JSON
// bodies ({"ok":true}) travel in the same packet either way.
const minCompressBytes = 512

// gzipWriters keeps a pool of compressors so a busy endpoint isn't allocating
// a 64KB deflate window per response.
var gzipWriters = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

// Compress gzips JSON responses for clients that accept it.
//
// The journal's list endpoints return highly repetitive JSON — the same field
// names once per entry — which gzips to a small fraction of its size. On a
// mobile connection that is the difference between one round trip and several,
// and it costs the server almost nothing at BestSpeed.
//
// Buffering is deliberate: the body is collected, and only compressed if it
// turns out to be worth it, which also keeps Content-Length accurate for the
// small responses that skip compression.
func Compress() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !acceptsGzip(c.GetHeader("Accept-Encoding")) {
			c.Next()
			return
		}

		buffered := &bufferedWriter{ResponseWriter: c.Writer}
		c.Writer = buffered
		c.Next()
		c.Writer = buffered.ResponseWriter

		body := buffered.body.Bytes()
		if len(body) == 0 {
			// No body to compress, but a status may still be pending — a 204
			// or an early abort would otherwise never reach the client.
			if buffered.status != 0 {
				buffered.ResponseWriter.WriteHeader(buffered.status)
			}
			return
		}

		header := buffered.ResponseWriter.Header()
		if len(body) < minCompressBytes || !compressibleType(header.Get("Content-Type")) {
			header.Set("Content-Length", strconv.Itoa(len(body)))
			buffered.ResponseWriter.WriteHeader(buffered.status)
			_, _ = buffered.ResponseWriter.Write(body)
			return
		}

		header.Set("Content-Encoding", "gzip")
		// The same URL can now answer with either encoding, so caches and
		// proxies must key on Accept-Encoding to avoid handing a gzipped body
		// to a client that did not ask for one.
		header.Add("Vary", "Accept-Encoding")
		header.Del("Content-Length") // length below refers to the raw body
		buffered.ResponseWriter.WriteHeader(buffered.status)

		gz := gzipWriters.Get().(*gzip.Writer)
		defer gzipWriters.Put(gz)
		gz.Reset(buffered.ResponseWriter)
		_, _ = gz.Write(body)
		_ = gz.Close()
	}
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]), "gzip") {
			return true
		}
	}
	return false
}

func compressibleType(contentType string) bool {
	switch {
	case strings.HasPrefix(contentType, "application/json"),
		strings.HasPrefix(contentType, "text/"),
		strings.HasPrefix(contentType, "application/javascript"),
		strings.HasPrefix(contentType, "image/svg+xml"):
		return true
	default:
		return false
	}
}

// bufferedWriter collects the response instead of streaming it, so Compress
// can decide after the handler has run whether gzip is worth it.
type bufferedWriter struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
}

func (w *bufferedWriter) WriteHeader(status int) {
	w.status = status
}

func (w *bufferedWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

func (w *bufferedWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *bufferedWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *bufferedWriter) Size() int { return w.body.Len() }

func (w *bufferedWriter) Written() bool { return w.status != 0 }

// Flush is a no-op: a buffered response has nothing to push yet, and letting
// it through would emit the body uncompressed and unbuffered. Streaming
// endpoints (none today) would need to opt out of this middleware.
func (w *bufferedWriter) Flush() {}
