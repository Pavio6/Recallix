package shared

import "net/http"

var (
	ErrBadRequest   = AppError{Code: "BAD_REQUEST", Message: "Invalid request", Status: http.StatusBadRequest}
	ErrUnauthorized  = AppError{Code: "UNAUTHORIZED", Message: "Authentication required", Status: http.StatusUnauthorized}
	ErrForbidden     = AppError{Code: "FORBIDDEN", Message: "Access denied", Status: http.StatusForbidden}
	ErrNotFound      = AppError{Code: "NOT_FOUND", Message: "Resource not found", Status: http.StatusNotFound}
	ErrConflict      = AppError{Code: "CONFLICT", Message: "Resource already exists", Status: http.StatusConflict}
	ErrInternal      = AppError{Code: "INTERNAL_ERROR", Message: "Internal server error", Status: http.StatusInternalServerError}
)

type AppError struct {
	Code    string
	Message string
	Status  int
	Err     error
}

func (e AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e AppError) Unwrap() error {
	return e.Err
}

func (e AppError) WithMessage(msg string) AppError {
	return AppError{Code: e.Code, Message: msg, Status: e.Status, Err: e.Err}
}

func (e AppError) WithError(err error) AppError {
	return AppError{Code: e.Code, Message: e.Message, Status: e.Status, Err: err}
}
