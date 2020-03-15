package main

import (
	"context"
	"flag"
	"github.com/buraksezer/olric/config"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/buraksezer/olric"
)

type OlricEmbedded struct {
	db  *olric.Olric
	log *log.Logger
	srv *http.Server
}

var usage = `Usage: 
  olric-embedded [flags] ...

This app aims to demonstrate embedded member deployment scenario. 
It also demonstrates how you deploy your app in Kubernetes environment.

Flags:
  -h                      
      Shows this screen.
  -p
      Sets port for HTTP server.`

func (e *OlricEmbedded) waitForInterrupt(idleConnsClosed chan struct{}) {
	shutDownChan := make(chan os.Signal, 1)
	signal.Notify(shutDownChan, syscall.SIGTERM, syscall.SIGINT)
	ch := <-shutDownChan
	e.log.Printf("[demo-app] Signal catched: %s", ch.String())

	if err := e.db.Shutdown(context.Background()); err != nil {
		e.log.Printf("[demo-app] Failed to shutdown Olric: %v", err)
	}

	// We received an interrupt signal, shut down.
	if err := e.srv.Shutdown(context.Background()); err != nil {
		// Error from closing listeners, or context timeout:
		log.Printf("HTTP server Shutdown: %v", err)
	}
	close(idleConnsClosed)
}

func main() {
	// No need for timestamp and etc in this function. Just log it.
	log.SetFlags(0)

	var addr string
	var port int
	var help bool

	// Parse command line parameters
	f := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	f.SetOutput(ioutil.Discard)
	f.BoolVar(&help, "h", false, "")
	f.StringVar(&addr, "-a", "127.0.0.1", "")
	f.IntVar(&port, "p", 8080, "")

	if err := f.Parse(os.Args[1:]); err != nil {
		log.Fatalf("demo-app: [ERROR] Failed to parse flags: %v", err)
	}

	if help {
		log.Printf(usage)
		return
	}

	////////////////////////////
	// The Demo App starts here!
	////////////////////////////

	l := log.New(os.Stdout, "", log.LstdFlags)

	// config.New returns a new config.Config with sane defaults. Available values for env:
	// local, lan, wan
	c := config.New("local")

	// Callback function. It's called when this node is ready to accept connections.
	ctx, cancel := context.WithCancel(context.Background())
	c.Started = func() {
		defer cancel()
		l.Println("demo-app: [INFO] Olric is ready to accept connections")
	}

	db, err := olric.New(c)
	if err != nil {
		l.Fatalf("demo-app: [ERROR] Failed to create Olric object: %v", err)
	}

	srv := &http.Server{
		Addr: net.JoinHostPort(addr, strconv.Itoa(port)),
	}
	e := OlricEmbedded{
		log: l,
		db:  db,
		srv: srv,
	}

	idleConnsClosed := make(chan struct{})
	go e.waitForInterrupt(idleConnsClosed)

	go func() {
		// Call Start at background. It's a blocker call.
		err = e.db.Start()
		if err != nil {
			e.log.Fatalf("demo-app: [ERROR] Failed to call Start: %v", err)
		}
	}()

	<-ctx.Done()

	err = e.srv.ListenAndServe()
	if err == http.ErrServerClosed {
		err = nil
	}
	if err != nil {
		log.Fatal(err)
	}
	<-idleConnsClosed
	e.log.Printf("demo-app: [INFO] Good bye!")
}
