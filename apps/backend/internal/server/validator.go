package server

import "github.com/go-playground/validator/v10"

// requestValidator adapts go-playground/validator to echo.Validator, so
// handlers can call echo.Context.Validate on bound request structs. Struct
// `validate` tags replace the TypeBox schemas used in the source app.
type requestValidator struct {
	v *validator.Validate
}

func newRequestValidator() *requestValidator {
	return &requestValidator{v: validator.New(validator.WithRequiredStructEnabled())}
}

func (rv *requestValidator) Validate(i any) error {
	return rv.v.Struct(i)
}
