package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/payments"
	"planeta_qosshy/routes"
	"planeta_qosshy/shop"
)

func main() {
	database.Connect()
	middleware.GetStore() // fail at startup (not on the first request) if COOKIE_SECRET is weak
	payments.MustConfigure()
	shop.MustConfigure()
	middleware.SetupLogger()
	log := middleware.Log()

	// ctx is cancelled by Ctrl+C or by SIGTERM — the signal `docker stop` sends.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Background job: cancels unpaid orders and returns their items to stock.
	var jobs sync.WaitGroup
	jobs.Add(1)
	go func() {
		defer jobs.Done()
		shop.RunExpiryLoop(ctx, database.DB)
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// r.Run() used before has no timeouts at all: a client that sends its headers one byte
	// a minute ("slowloris") would hold a connection forever, and thousands of them would
	// take the server down.
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           routes.SetupRouter(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second, // the whole request, incl. a 16 MB upload on mobile
		WriteTimeout:      60 * time.Second, // (websockets clear these after the upgrade)
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Infof("listening on %s", srv.Addr)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err) // e.g. the port is already taken
		}
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting new connections, let requests in progress finish
	// (a buyer in the middle of checkout gets an answer, not a broken connection),
	// stop the background job, close the DB pool.
	log.Info("shutting down: finishing requests in progress (at most 15 s)")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warnf("shutdown: %v", err)
	}
	stop() // in case the server stopped by itself: end the expiry loop too
	jobs.Wait()
	database.Close()
	log.Info("stopped")
}
