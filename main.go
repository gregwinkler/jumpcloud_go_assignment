package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gregwinkler/jumpcloud_go_assignment/handlers"
)

func main() {
	// router := handlers.InitializeRoutes()
	//	log.Fatal(http.ListenAndServe(":8080", router))
	listenAddr := ":8080"

	logger := log.New(os.Stdout, "http: ", log.LstdFlags)

	done := make(chan bool, 1)

	server := newWebserver(logger, listenAddr)
	go gracefullShutdown(server, logger, handlers.ShutdownChan, done)

	logger.Println("Server is ready to handle requests at", listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Could not listen on %s: %v\n", listenAddr, err)
	}

	<-done

	logger.Println("Server stopped")

}

func gracefullShutdown(server *http.Server, logger *log.Logger, quit <-chan bool, done chan<- bool) {
	<-quit

	logger.Println("Server is shutting down...")

	<-handlers.ShutdownChan // wait for everyone to be done on the processing side

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	server.SetKeepAlivesEnabled(false)
	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("Could not gracefully shutdown the server: %v\n", err)
	}

	close(done)
}

func newWebserver(logger *log.Logger, listenAddr string) *http.Server {
	router := handlers.InitializeRoutes()

	return &http.Server{
		Addr:         listenAddr,
		Handler:      router,
		ErrorLog:     logger,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}
}
