# Vermouth

A simple HTTP library made for education purposes, it supports middlewares, grouping, params and more!

`Vermouth` is built entirely separated from `net/http`, so if you want an alternative that it built around it, you could check [Vodka](https://github.com/Jamlie/vodka), [Gin](https://github.com/gin-gonic/gin), [Echo](https://github.com/labstack/echo), [Chi](https://github.com/go-chi/chi)

## Install

```sh
go get github.com/Jamlie/vermouth
```

## Example

```go
package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/Jamlie/vermouth"
)

func main() {
	s := vermouth.New()

	s.GET("/", func(c vermouth.Context) error {
		return c.File(path.Join("frontend", "index.html"), vermouth.StatusOK)
	})

	s.GET("/static/:file", func(c vermouth.Context) error {
		filename := c.Params("file")
		file, err := os.ReadFile(path.Join("frontend", filename))
		if err != nil {
			return err
		}

		_, err = c.Write(file)
		return err
	})

	userG := s.Group("/user")

	userG.GET("/hello", func(c vermouth.Context) error {
		_, err := c.HTML("<h1>user HEllo</h1>", 200)
		return err
	})

	userG.GET("/:id", func(c vermouth.Context) error {
		id := c.Params("id")
		resp := fmt.Sprintf("User ID: %s", id)
		_, err := c.String(resp, 200)
		return err
	})

	userG.GET("/:id/:name", func(c vermouth.Context) error {
		id := c.Params("id")
		name := c.Params("name")
		resp := fmt.Sprintf("User ID: %s %s", id, name)
		_, err := c.String(resp, 200)
		return err
	})

	s.POST("/json", func(c vermouth.Context) error {
		type Data struct {
			Random string `json:"random"`
			Name   string `json:"name"`
		}

		data := Data{
			Random: "hedioasjda",
			Name:   "Jamlie",
		}

		return c.JSON(data, vermouth.StatusOK)
	})

	log.Fatal(s.Start(":8080"))
}
```
