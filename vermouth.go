package vermouth

import (
	"io"
	"log"
	"log/slog"
	"net"
	"strings"
)

type HandlerFunc func(c Context) error

type Vermouth struct {
	paths   map[string]HandlerFunc
	context Context
}

func NewVermouth() *Vermouth {
	return &Vermouth{
		paths:   make(map[string]HandlerFunc),
		context: newCtx(),
	}
}

func (s *Vermouth) HandleFunc(path string, fn HandlerFunc) {
	if _, ok := s.paths[path]; !ok {
		s.paths[path] = fn
	}
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
				s.context.setPlatform(platformLine[1][1 : len(platformLine[1])-1])
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

	if handler, ok := s.paths[path]; ok {
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

func (s *Vermouth) Start(port string) {
	l, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}

		s.context.setConn(conn)

		go func() {
			defer conn.Close()
			s.ServeHTTP()
		}()
	}
}
