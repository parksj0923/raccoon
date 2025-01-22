package fiberhelpers

import (
	"autotrader/main/errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
)

func RequestParse[T any](context *fiber.Ctx) T {
	var destination T
	if err := context.BodyParser(&destination); err != nil {
		typeName := reflect.TypeOf(destination).Name()
		log.Error(err.Error())
		panic(errors.NewRequestParserError(typeName))
	}
	return destination
}

func ListenWithGraceFullyShutdown(app *fiber.App, port string) {
	if !strings.ContainsAny(port, ":") {
		port = fmt.Sprintf(":%s", port)
	}

	c := make(chan os.Signal, 1)
	serverShutdown := make(chan bool)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		_ = <-c
		log.Info("Gracefully shutting down...")
		_ = app.Shutdown()
		serverShutdown <- true
	}()

	address := "0.0.0.0" + port
	log.Infof("Starting server on %s", address)
	err := app.Listen(address)
	if err != nil {
		log.Errorf("Server failed to start on %s: %v", address, err)
	}
	<-serverShutdown
}
