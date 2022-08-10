package handler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
)

// Error level definition
type ErrLevel string

const (
	FATAL ErrLevel = "FATAL"
	ERROR ErrLevel = "ERROR"
	WARN  ErrLevel = "WARN"
	INFO  ErrLevel = "INFO"
)

// Custom error definition
type CustomError struct {
	Code        int         `json:"code"`
	Level       ErrLevel    `json:"level"`
	Message     interface{} `json:"message"`
	ErrorDetail error       `json:"error"`
}

func (c *CustomError) Error() string {
	return fmt.Sprintf("code=%d level=%s message=%s err=%s", c.Code, c.Level, c.Message, c.ErrorDetail)
}

type Context struct {
	echo.Context
}

func (c *Context) BindValidate(i interface{}) error {
	if err := c.Bind(i); err != nil {
		return &CustomError{
			Code:        http.StatusBadRequest,
			Level:       WARN,
			Message:     "request bind failed",
			ErrorDetail: err,
		}
	}
	if err := c.Validate(i); err != nil {
		return &CustomError{
			Code:        http.StatusBadRequest,
			Level:       WARN,
			Message:     "request validation failed",
			ErrorDetail: err,
		}
	}
	return nil
}
