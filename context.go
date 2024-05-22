package vermouth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/url"
	"os"
)

var headersError = errors.New("no header exists")

type (
	Context interface {
		String(string, int) (int, error)
		HTML(string, int) (int, error)
		JSON(any, int) error
		Err404() (int, error)
		File(string, int) error
		Method() string
		Host() string
		Platform() string
		UserAgent() string
		Accept() string
		SetHeader(string, string)
		Write([]byte) (int, error)
		Read([]byte) (int, error)
		Body() io.ReadCloser
		ParseJSON(any) error
		ParseForm() (url.Values, error)
		Redirect(string) error
		Params(string) string
		Context() context.Context

		setMethod(string)
		getConn() net.Conn
		setConn(net.Conn)
		setHost(string)
		setPlatform(string)
		setUserAgent(string)
		setAccept(string)
		setBody(io.ReadCloser)
		setParams(map[string]string)
	}

	ctx struct {
		conn                net.Conn
		method              string
		hostname            string
		platform            string
		userAgent           string
		accept              string
		headers             map[string]string
		didWriteHTTP1Header bool
		body                io.ReadCloser
		params              map[string]string
		ctx                 context.Context
	}
)

func newCtx() *ctx {
	return &ctx{
		headers: make(map[string]string),
	}
}

func (c *ctx) Write(b []byte) (n int, err error) {
	err = c.writeHeaders()
	if err == headersError {
		if !c.didWriteHTTP1Header {
			err = c.writeHTTP1Header(StatusOK)
			if err != nil {
				log.Fatal("Could not write HTTP 1.1 header", err)
			}
		}

		data := string(b)
		c.SetHeader("Content-Length", fmt.Sprint(len(data)))
		err = c.writeHeaders()
	}
	if err != nil {
		log.Fatal(err)
	}

	return c.conn.Write(b)
}

func (c *ctx) Read(p []byte) (n int, err error) {
	if c.conn == nil {
		return 0, errors.New("no active connection")
	}

	n, err = c.conn.Read(p)
	if err != nil {
		return n, err
	}

	return n, nil
}

func (c *ctx) ParseJSON(v any) error {
	if c.body == nil {
		return errors.New("No request body to read")
	}

	defer c.body.Close()
	return json.NewDecoder(c.body).Decode(v)
}

func (c *ctx) ParseForm() (url.Values, error) {
	if c.body == nil {
		return nil, errors.New("No request body to read")
	}

	defer c.body.Close()

	bodyBytes, err := io.ReadAll(c.body)
	if err != nil {
		return nil, err
	}

	return url.ParseQuery(string(bodyBytes))
}

func (c *ctx) SetHeader(key, value string) {
	c.headers[key] = value
}

func (c *ctx) String(plain string, statusCode int) (n int, err error) {
	err = c.writeHTTP1Header(statusCode)
	if err != nil {
		log.Fatal("Could not write HTTP 1.1 header", err)
	}
	c.SetHeader("Content-Type", "text/plain")
	c.SetHeader("Content-Length", fmt.Sprintf("%d", len(plain)))

	return c.Write([]byte(plain))
}

func (c *ctx) HTML(html string, statusCode int) (n int, err error) {
	err = c.writeHTTP1Header(statusCode)
	if err != nil {
		log.Fatal("Could not write HTTP header", err)
	}
	c.SetHeader("Content-Type", "text/html")
	c.SetHeader("Content-Length", fmt.Sprintf("%d", len(html)))

	return c.Write([]byte(html))
}

func (c *ctx) JSON(data any, statusCode int) error {
	err := c.writeHTTP1Header(statusCode)
	if err != nil {
		log.Fatal("Could not write HTTP 1.1 header", err)
	}
	c.SetHeader("Content-Type", "application/json")
	jsonRes, err := json.Marshal(data)
	if err != nil {
		slog.Error(err.Error())
		return err
	}

	c.SetHeader("Content-Length", fmt.Sprintf("%d", len(string(jsonRes))))
	_, err = c.Write(jsonRes)
	return err
}

func (c *ctx) Redirect(path string) error {
	err := c.writeHTTP1Header(StatusFound)
	if err != nil {
		log.Fatal("Could not write HTTP 1.1 header", err)
	}
	c.SetHeader("Location", path)
	err = c.writeHeaders()
	if err != nil {
		log.Fatal("Could not write HTTP headers", err)
	}

	return c.conn.Close()
}

func (c *ctx) Err404() (n int, err error) {
	html := `<h1>Error 404 Not Found</h1>`

	err = c.writeHTTP1Header(StatusNotFound)
	if err != nil {
		log.Fatal("Could not write HTTP header", err)
	}
	c.SetHeader("Content-Type", "text/html")
	c.SetHeader("Content-Length", fmt.Sprintf("%d", len(html)))
	return c.Write([]byte(html))
}

func (c *ctx) File(path string, statusCode int) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	err = c.writeHTTP1Header(statusCode)
	if err != nil {
		log.Fatal("Could not write HTTP 1.1 header", err)
	}

	c.SetHeader("Content-Type", "text/html")
	c.SetHeader("Content-Length", fmt.Sprintf("%d", len(file)))
	_, err = c.Write(file)
	return err
}

func (c *ctx) writeHTTP1Header(statusCode int) error {
	statusCodeString := StatusText(statusCode)

	_, err := c.conn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, statusCodeString)))
	if err != nil {
		c.didWriteHTTP1Header = false
		return err
	}

	c.didWriteHTTP1Header = true
	return nil
}

func (c *ctx) writeHeaders() error {
	if len(c.headers) == 0 {
		return headersError
	}

	for key, value := range c.headers {
		_, err := c.conn.Write([]byte(fmt.Sprintf("%s: %s\r\n", key, value)))
		if err != nil {
			return err
		}
	}

	_, err := c.conn.Write([]byte("\r\n"))
	return err
}

func (c *ctx) Context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func (c *ctx) Platform() string {
	return c.platform
}

func (c *ctx) UserAgent() string {
	return c.userAgent
}

func (c *ctx) Method() string {
	return c.method
}

func (c *ctx) Host() string {
	return c.hostname
}

func (c *ctx) Accept() string {
	return c.accept
}

func (c *ctx) Body() io.ReadCloser {
	return c.body
}

func (c *ctx) Params(key string) string {
	return c.params[key]
}

func (c *ctx) getConn() net.Conn {
	return c.conn
}

func (c *ctx) setConn(conn net.Conn) {
	c.conn = conn
}

func (c *ctx) setMethod(method string) {
	c.method = method
}

func (c *ctx) setHost(hostname string) {
	c.hostname = hostname
}

func (c *ctx) setPlatform(platform string) {
	c.platform = platform
}

func (c *ctx) setUserAgent(userAgent string) {
	c.userAgent = userAgent
}

func (c *ctx) setAccept(accept string) {
	c.accept = accept
}

func (c *ctx) setBody(body io.ReadCloser) {
	c.body = body
}

func (c *ctx) setParams(params map[string]string) {
	c.params = params
}
