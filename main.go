package main

import (
	"os"
	"os/signal"
	"raccoon/bot"
	"raccoon/utils/log"
	"syscall"
	"time"
)

func main() {
	// 1) API 키
	apiKey := os.Getenv("UPBIT_ACCESS_KEY")
	secretKey := os.Getenv("UPBIT_SECRET_KEY")
	pairs := []string{"KRW-DOGE"}

	// 2) Raccoon 인스턴스 생성
	raccoon, err := bot.NewRaccoon(apiKey, secretKey, pairs)
	if err != nil {
		log.Fatal(err)
	}

	// 4) Start
	raccoon.Start()

	// 5) OS 시그널 대기 (Graceful Stop)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Infof("Shutting down gracefully...")

	// 6) Stop
	raccoon.Stop()
	time.Sleep(1 * time.Second)
	log.Infof("Shutdown complete.")
}
