package fiberhelpers

import (
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"runtime/debug"
	"time"
)

type LogEntry struct {
	Message    string `json:"message"`
	StackTrace string `json:"stack_trace,omitempty"`
}

func NewRecover() fiber.Handler {
	return recover.New(
		recover.Config{
			StackTraceHandler: func(c *fiber.Ctx, e interface{}) {
				stackTrace := debug.Stack()
				logEntry := LogEntry{
					Message:    fmt.Sprintf("%v", e),
					StackTrace: string(stackTrace),
				}
				logJSON, _ := json.Marshal(logEntry)

				fmt.Printf("%s | %s", time.Now().Format("2006-01-02 15:04:05"), logJSON)

			},
			EnableStackTrace: true,
		},
	)
}
