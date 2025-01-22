package response

import (
	"autotrader/main/errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"strconv"
)

const (
	RequestError = "The request is not valid."
)

func getMessage(errCode string) string {
	if v, ok := errors.ErrorMessages[errCode]; ok {
		return v
	}
	return RequestError
}

type Ext struct {
	*fiber.Ctx
}

// Ok : 성공(200) 응답
func (ext Ext) Ok(data interface{}) error {
	return ext.Status(fiber.StatusOK).JSON(data)
}

// Error : 에러 응답
// - err: 실제 Go 에러 객체
// - errCode: (옵션) Upbit 오류 코드나 HTTP StatusCode 등을 string으로 넘길 수 있음
func (ext Ext) Error(err error, errCode ...string) error {
	var code string
	if len(errCode) > 0 {
		code = errCode[0] // 사용자가 명시한 코드를 우선
	} else {
		// 사용자가 명시 안 했으면 err.Error() 문자열을 코드로 사용
		code = err.Error()
	}

	// Upbit 에러 메시지 찾기
	msg := errors.GetErrorMessage(code)

	// 기본 상태코드는 400 Bad Request (또는 비즈니스 로직 따라 조정)
	status := fiber.StatusBadRequest

	// 특정 오류 코드를 만나면 401 등 다른 HTTP 상태코드로 보낼 수도 있음
	switch code {
	case errors.ErrInvalidQueryPayload,
		errors.ErrJWTVerification,
		errors.ErrExpiredAccessKey,
		errors.ErrNonceUsed,
		errors.ErrNoAuthorizationIP,
		errors.ErrOutOfScope:
		status = fiber.StatusUnauthorized

		// TODO: 다른 에러 코드 -> 다른 Status Code 매핑할 수 있음
		// case errors.ErrSomeOtherUpbitError:
		// 	status = fiber.StatusForbidden
	}

	res := errors.ErrorResponse{
		Code:    strconv.Itoa(status),
		Message: msg,
	}

	return ext.Status(status).JSON(res)
}

// Panic : 서버 내부 에러 (500) 응답
func (ext Ext) Panic(id interface{}) error {
	fmt.Printf("[PANIC] %v\n", id)
	res := errors.ErrorResponse{
		Code:    strconv.Itoa(fiber.StatusInternalServerError),
		Message: "Internal Server Error",
	}
	return ext.Status(fiber.StatusInternalServerError).JSON(res)
}

// Forbidden : 권한 부족 등 403 응답
func (ext Ext) Forbidden(err error, errCode ...string) error {
	var code string
	if len(errCode) > 0 {
		code = errCode[0]
	} else {
		code = err.Error()
	}

	msg := errors.GetErrorMessage(code)

	res := errors.ErrorResponse{
		Code:    strconv.Itoa(fiber.StatusForbidden),
		Message: msg,
	}
	return ext.Status(fiber.StatusForbidden).JSON(res)
}
