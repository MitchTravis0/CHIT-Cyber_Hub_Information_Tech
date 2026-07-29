package core

import "fmt"

// Error codes carried by JobError and by UserError. Keep the list short: the
// frontend only uses them to pick an icon, the message does the explaining.
const (
	CodeInvalidInput = "invalid_input"
	CodeNetwork      = "network"
	CodeTimeout      = "timeout"
	CodePermission   = "permission"
	CodeNotFound     = "not_found"
	CodeInternal     = "internal"
)

// UserError carries a message written for a junior tech, not a developer.
// Bound methods and job functions should return one of these so the message
// can be shown verbatim in the UI.
type UserError struct {
	Code    string
	Message string
}

func (e *UserError) Error() string { return e.Message }

func Errorf(code, format string, args ...any) error {
	return &UserError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// CodeOf reports the error code of err, or CodeInternal for plain errors.
func CodeOf(err error) string {
	if ue, ok := err.(*UserError); ok {
		return ue.Code
	}
	return CodeInternal
}

// MessageOf returns a message safe to show a user. Plain errors (raw stdlib
// text like "dial udp: i/o timeout") are replaced with a generic line so the
// UI never leaks developer wording.
func MessageOf(err error) string {
	if ue, ok := err.(*UserError); ok {
		return ue.Message
	}
	return "Something went wrong and the operation stopped. Try again, and check the log if it keeps happening."
}
