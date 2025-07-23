package vermouth

import (
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
)

type (
	HandlerFunc    func(c Context) error
	MiddlewareFunc func(HandlerFunc) HandlerFunc

	route struct {
		pattern string
		handler HandlerFunc
		method  string
	}

	Vermouth struct {
		routes       []route
		middlewares  []MiddlewareFunc
		mu           sync.Mutex
		patternGroup string
	}
)

func New() *Vermouth {
	return &Vermouth{
		routes:      []route{},
		middlewares: []MiddlewareFunc{},
	}
}

func (v *Vermouth) Group(pattern string) *Vermouth {
	if pattern[0] != '/' {
		panic("Route must start with /")
	}

	if pattern[len(pattern)-1] == '/' {
		pattern = pattern[:len(pattern)-1]
	}

	v.patternGroup = pattern

	return v
}

func (v *Vermouth) Use(mw MiddlewareFunc) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.middlewares = append(v.middlewares, mw)
}

func (v *Vermouth) GET(pattern string, fn HandlerFunc) {
	pattern = v.patternGroup + pattern
	v.handleFunc(MethodGet, pattern, fn)
}

func (v *Vermouth) POST(pattern string, fn HandlerFunc) {
	pattern = v.patternGroup + pattern
	v.handleFunc(MethodPost, pattern, fn)
}

func (v *Vermouth) PUT(pattern string, fn HandlerFunc) {
	pattern = v.patternGroup + pattern
	v.handleFunc(MethodPut, pattern, fn)
}

func (v *Vermouth) DELETE(pattern string, fn HandlerFunc) {
	pattern = v.patternGroup + pattern
	v.handleFunc(MethodDelete, pattern, fn)
}

func (v *Vermouth) PATCH(pattern string, fn HandlerFunc) {
	pattern = v.patternGroup + pattern
	v.handleFunc(MethodPatch, pattern, fn)
}

func (v *Vermouth) handleFunc(method, pattern string, fn HandlerFunc) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.routes = append(v.routes, route{pattern, fn, method})
}

func (v *Vermouth) ServeHTTP(conn net.Conn) error {
	defer conn.Close()
	data := make([]byte, 1024)
	n, err := conn.Read(data)
	if err != nil {
		return err
	}

	connection := strings.Split(string(data[:n]), "\r\n")
	reqLine := strings.Split(connection[0], " ")

	if len(reqLine) < 2 {
		return errors.New("The request does not have a method not a path")
	}

	method, path := reqLine[0], reqLine[1]

	context := newCtx()
	context.setConn(conn)
	context.setMethod(method)

	if len(connection) > 1 {
		if context.Host() == "" {
			hostLine := strings.Split(connection[1], " ")
			if len(hostLine) > 1 {
				context.setHost(hostLine[1])
			}
		}
	}

	bodyIndex := -1
	for i, line := range connection {
		if line == "" {
			bodyIndex = i + 1
			break
		}
	}
	if bodyIndex != -1 && len(connection) > bodyIndex {
		context.setBody(io.NopCloser(
			strings.NewReader(strings.Join(connection[bodyIndex:], "\r\n")),
		))
	}

	if len(connection) > 6 {
		if context.Platform() == "" {
			platformLine := strings.Split(connection[6], " ")
			if len(platformLine) > 1 {
				context.setPlatform(platformLine[1])
			}
		}
	}

	if len(connection) > 9 {
		if context.UserAgent() == "" {
			userAgentLine := strings.Split(connection[9], " ")
			if len(userAgentLine) > 1 {
				context.setUserAgent(userAgentLine[1])
			}
		}
	}

	if len(connection) > 10 {
		if context.Accept() == "" {
			acceptLine := strings.Split(connection[10], " ")
			if len(acceptLine) > 1 {
				context.setAccept(acceptLine[1])
			}
		}
	}

	var handler HandlerFunc
	var params map[string]string

	v.mu.Lock()
	for _, route := range v.routes {
		if matchedParams, ok := matchRoute(route.pattern, path); ok && route.method == method {
			handler = route.handler
			params = matchedParams
			break
		}
	}
	v.mu.Unlock()

	if handler != nil {
		for i := len(v.middlewares) - 1; i >= 0; i-- {
			handler = v.middlewares[i](handler)
		}

		context.setParams(params)
		err := handler(context)
		return err
	}
	_, err = context.Err404()
	return err
}

func (v *Vermouth) Static(prefix, directory string) {
	v.GET(prefix+"/:filepath", func(c Context) error {
		rawPath := c.Params("filepath")

		relPath := filepath.Clean(rawPath)
		if strings.Contains(relPath, "..") {
			_, err := c.Err404()
			return err
		}

		fullPath := filepath.Join(directory, relPath)

		return c.File(fullPath, 200)
	})
}

func matchRoute(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	params := make(map[string]string)

	for i, part := range patternParts {
		if part == "" {
			continue
		}

		if strings.HasPrefix(part, ":") {
			if strings.HasSuffix(part, "*") {
				key := strings.TrimSuffix(part[1:], "*")
				params[key] = strings.Join(pathParts[i:], "/")
				return params, true
			}

			if i >= len(pathParts) {
				return nil, false
			}

			params[part[1:]] = pathParts[i]
		} else {
			if i >= len(pathParts) || part != pathParts[i] {
				return nil, false
			}
		}
	}

	if len(patternParts) != len(pathParts) {
		return nil, false
	}

	return params, true
}

func (v *Vermouth) Start(port string) error {
	errch := make(chan error)
	l, err := net.Listen("tcp", port)
	if err != nil {
		return err
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		go func(conn net.Conn) {
			err := v.ServeHTTP(conn)
			if err != nil {
				errch <- err
			} else {
				errch <- nil
			}
		}(conn)

		select {
		case err := <-errch:
			if err != nil {
				return err
			}
		default:
		}
	}
}
