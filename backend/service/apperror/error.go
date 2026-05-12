package apperror

import "net/http"

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Message
}

func New(status int, code string, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func Unauthorized(message string) *Error {
	return New(http.StatusUnauthorized, "UNAUTHORIZED", message)
}

func Forbidden(code string, message string) *Error {
	return New(http.StatusForbidden, code, message)
}

func NotFound(message string) *Error {
	return New(http.StatusNotFound, "NOT_FOUND", message)
}

func Validation(message string) *Error {
	return New(http.StatusBadRequest, "VALIDATION_ERROR", message)
}

func Conflict(code string, message string) *Error {
	return New(http.StatusConflict, code, message)
}

func Internal(message string) *Error {
	return New(http.StatusInternalServerError, "INTERNAL_ERROR", message)
}

func Busy(message string) *Error {
	return New(http.StatusServiceUnavailable, "BUSY", message)
}
