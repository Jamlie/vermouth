package vermouth

import (
	"io"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
)

type HandlerFunc func(c Context) error

type route struct {
	pattern string
	handler HandlerFunc
	method  string
}

type Vermouth struct {
	routes []route
	mu     sync.Mutex
}

func New() *Vermouth {
	return &Vermouth{
		routes: []route{},
	}
}

func (v *Vermouth) GET(pattern string, fn HandlerFunc) {
	v.handleFunc(MethodGet, pattern, fn)
}

func (v *Vermouth) POST(pattern string, fn HandlerFunc) {
	v.handleFunc(MethodPost, pattern, fn)
}

func (v *Vermouth) PUT(pattern string, fn HandlerFunc) {
	v.handleFunc(MethodPut, pattern, fn)
}

func (v *Vermouth) DELETE(pattern string, fn HandlerFunc) {
	v.handleFunc(MethodDelete, pattern, fn)
}

func (v *Vermouth) PATCH(pattern string, fn HandlerFunc) {
	v.handleFunc(MethodPatch, pattern, fn)
}

func (v *Vermouth) handleFunc(method, pattern string, fn HandlerFunc) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.routes = append(v.routes, route{pattern, fn, method})
}

func (v *Vermouth) ServeHTTP(conn net.Conn) {
	defer conn.Close()
	data := make([]byte, 1024)
	n, err := conn.Read(data)
	if err != nil {
		log.Println(err)
		return
	}

	connection := strings.Split(string(data[:n]), "\r\n")
	reqLine := strings.Split(connection[0], " ")

	if len(reqLine) < 2 {
		return
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

	for _, route := range v.routes {
		if matchedParams, ok := matchRoute(route.pattern, path); ok && route.method == method {
			handler = route.handler
			params = matchedParams
			break
		}
	}

	if handler != nil {
		context.setParams(params)
		err := handler(context)
		if err != nil {
			slog.Error("Error handling the endpoint", err)
		}
	} else {
		_, err := context.Err404()
		if err != nil {
			slog.Error("Error", err)
		}
	}
}

func matchRoute(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) != len(pathParts) {
		return nil, false
	}

	params := make(map[string]string)
	for i, part := range patternParts {
		if strings.HasPrefix(part, ":") {
			params[part[1:]] = pathParts[i]
		} else if part != pathParts[i] {
			return nil, false
		}
	}

	return params, true
}

func (v *Vermouth) Start(port string) error {
	l, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		go v.ServeHTTP(conn)
	}
}
