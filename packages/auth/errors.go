package auth

import "fmt"

type ErrorCode string

const (
	ErrorCodeUnauthorized ErrorCode = "unauthorized"
	ErrorCodeForbidden    ErrorCode = "forbidden"
)

type ErrUnauthorized string

func (e ErrUnauthorized) Error() string { return string(e) }

type ErrForbidden string

func (e ErrForbidden) Error() string { return string(e) }

func Unauthorized(reason string) error {
	return ErrUnauthorized(reason)
}

func Forbidden(reason string) error {
	return ErrForbidden(reason)
}

func WrapVerificationError(err error) error {
	return fmt.Errorf("verify auth token: %w", err)
}
