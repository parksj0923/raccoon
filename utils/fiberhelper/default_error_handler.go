package fiberhelpers

import (
	"autotrader/main/errors"
	_error "errors"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
)

func DefaultErrorHandler(ctx *fiber.Ctx, err error) error {
	errorBase, err := errors.ConvertToErrorBase(err)
	if err != nil {
		fiberError, err := convertToFiberError(err)
		if err != nil {
			log.Error(err.Error())
			return ctx.Status(fiber.StatusInternalServerError).JSON(errors.NewInternalServerError())
		} else {
			log.Error(fiberError.Error())
			return ctx.Status(fiberError.Code).JSON(errors.NewInternalServerError())
		}
	}
	err = ctx.Status(errorBase.Status).JSON(errorBase.NewErrorResponse())
	if err != nil {
		log.Error(err.Error())
		return ctx.Status(fiber.StatusInternalServerError).JSON(errors.NewInternalServerError())
	}
	return nil
}

func convertToFiberError(err error) (fiber.Error, error) {
	var fiberError *fiber.Error
	converted := _error.As(err, &fiberError)
	if converted {
		return *fiberError, nil
	}
	return fiber.Error{}, err
}
