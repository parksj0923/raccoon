package middleware

import (
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"strings"
)

func TokenValidationMiddleware() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		return ctx.Next()
	}
}

func LogMiddleware(skipPath ...string) fiber.Handler {
	customTags := map[string]logger.LogFunc{
		"requestBody": getRequestBody(),
	}

	return logger.New(logger.Config{
		TimeFormat: "2006-01-02 15:04:05",
		Format:     "${time} | ${status} | ${latency} | ${method} | ${path} | Query: ${queryParams} | Body: ${requestBody}\n",
		Next: func(c *fiber.Ctx) bool {
			// Skip the middleware if the request path
			for _, p := range skipPath {
				if c.Path() == p {
					return true
				}
			}
			return false
		},
		CustomTags: customTags,
	})
}

func getRequestBody() logger.LogFunc {
	return func(output logger.Buffer, c *fiber.Ctx, data *logger.Data, extraParam string) (int, error) {
		var requestBody map[string]interface{}
		if c.Get("Content-Type") != "multipart/form-data" {
			err := json.Unmarshal(c.Body(), &requestBody)
			if err == nil {
				body := strings.TrimSpace(string(c.Body()))
				body = strings.ReplaceAll(body, "\n", "")
				return output.WriteString(fmt.Sprintf("%v", body))
			}
		}
		return output.WriteString("")
	}
}
