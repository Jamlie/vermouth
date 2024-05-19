package vermouth

import (
	"io"
	"log"
	"log/slog"
	"net"
	"strings"
)

type HandlerFunc func(c Context) error

type route struct {
	pattern string
	handler HandlerFunc
}

type Vermouth struct {
	routes  []route
	context Context
}

func New() *Vermouth {
	return &Vermouth{
		routes:  []route{},
		context: newCtx(),
	}
}

func (s *Vermouth) HandleFunc(pattern string, fn HandlerFunc) {
	s.routes = append(s.routes, route{pattern, fn})
}

func (s *Vermouth) ServeHTTP() {
	data := make([]byte, 1024)
	n, err := s.context.getConn().Read(data)
	if err != nil {
		log.Println(err)
		return
	}

	connection := strings.Split(string(data[:n]), "\r\n")
	reqLine := strings.Split(connection[0], " ")

	method, path := reqLine[0], reqLine[1]
	s.context.setMethod(method)

	if len(reqLine) < 2 {
		_, _ = s.context.Err404()
		return
	}

	if len(connection) > 1 {
		if s.context.Host() == "" {
			hostLine := strings.Split(connection[1], " ")
			if len(hostLine) > 1 {
				s.context.setHost(hostLine[1])
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
		s.context.setBody(io.NopCloser(
			strings.NewReader(strings.Join(connection[bodyIndex:], "\r\n")),
		))
	}

	if len(connection) > 6 {
		if s.context.Platform() == "" {
			platformLine := strings.Split(connection[6], " ")
			if len(platformLine) > 1 {
				s.context.setPlatform(platformLine[1])
			}
		}
	}

	if len(connection) > 9 {
		if s.context.UserAgent() == "" {
			userAgentLine := strings.Split(connection[9], " ")
			if len(userAgentLine) > 1 {
				s.context.setUserAgent(userAgentLine[1])
			}
		}
	}

	if len(connection) > 10 {
		if s.context.Accept() == "" {
			acceptLine := strings.Split(connection[10], " ")
			if len(acceptLine) > 1 {
				s.context.setAccept(acceptLine[1])
			}
		}
	}

	var handler HandlerFunc
	var params map[string]string

	for _, route := range s.routes {
		if matchedParams, ok := matchRoute(route.pattern, path); ok {
			handler = route.handler
			params = matchedParams
			break
		}
	}

	if handler != nil {
		s.context.setParams(params)
		err := handler(s.context)
		if err != nil {
			slog.Error("Error handling the endpoint", err)
		}
	} else {
		_, err := s.context.Err404()
		if err != nil {
			slog.Error("Error", err)
		}
	}
}

func matchRoute(pattern, path string) (map[string]string, bool) {
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	if len(patternParts) > 0 && pathParts[len(pathParts)-1] == "" {
		return nil, false
	}

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

func (s *Vermouth) Start(port string) error {
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

		s.context.setConn(conn)

		go func() {
			defer conn.Close()
			s.ServeHTTP()
		}()
	}
}
