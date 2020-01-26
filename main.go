package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"
)

// нам понадобится именно RWMutex, т.к. изначальный лок для предварительных
// шагов будет приоритетным, остальные локи будут только не чтение
var startLocker = &sync.RWMutex{}

func main() {
	// поставим приоритетный лок на запись
	startLocker.Lock()
	go func() {
		// по окончанию наших предварительных шагов снимем лок на запись
		defer startLocker.Unlock()
		// здесь мы можем выполнить все наши предварительные шаги:
		// подлючение к базе, формирование шаблонов и т.д.
	}()

	router := http.NewServeMux()
	router.HandleFunc("/", mainHandler)
	server := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: router,
	}

	done := make(chan bool)
	go shutDown(server, done)

	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		panic(fmt.Sprintf("server.ListenAndServe: %s", err))
	}
	<-done
	if err == http.ErrServerClosed {
		fmt.Printf("server.ListenAndServe: %s\n", err)
	}
}

func shutDown(server *http.Server, done chan<- bool) {
	breakSignals := map[string]bool{
		"hangup":     true, // kill -HUP <pid>
		"interrupt":  true, // kill -2 <pid> или CTRL+C
		"terminated": true, // kill <pid>
	}
	quit := make(chan os.Signal)
	signal.Notify(quit)
	var sg os.Signal
	for {
		sg = <-quit
		fmt.Printf("os.Signal: %s\n", sg)
		if breakSignals[sg.String()] {
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.SetKeepAlivesEnabled(false)
	fmt.Println("server.Shutdown")
	if err := server.Shutdown(ctx); err != nil {
		// здесь, скорее всего, context timeout првевышен
		fmt.Printf("server.Shutdown: %s\n", err)
	}
	if sg.String() == "hangup" {
		// при получении сигнала hangup,
		// запустим наш сервис
		cmd := exec.Command("./main")
		if err := cmd.Start(); err != nil {
			fmt.Printf("cmd.Start: %s\n", err)
		}
	}
	close(done)
}

func mainHandler(response http.ResponseWriter, request *http.Request) {
	defer func() {
		defer func() { recover() }() // вдруг мы допустим какую-то ошибку при обработке ошибок )
		if err := recover(); err != nil {
			// обработаем ошибку корректно, уведомив об этом пользователя
			response.WriteHeader(http.StatusInternalServerError)
			// здесь можно подготовить вывод ошибки в нужном оформлении,
			// чтобы все выглядело гармонично с дизайном нашего веб сервиса
			outData := "Error occurred"
			response.Write([]byte(outData))
		}
	}()
	startLocker.RLock()
	defer startLocker.RUnlock()
	request.Close = true
	outData := "It Works!"
	response.WriteHeader(http.StatusOK)
	response.Write([]byte(outData))
}
