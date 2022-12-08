package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var (
	listenFlag = flag.String("listen", ":5678", "address and port to listen")
	textFlag   = flag.String("text", "", "text to put on the webpage")
	envFlag    = flag.String("env", "", "environment variable to echo to the webpage")

	// stdoutW and stderrW are for overriding in test.
	stdoutW = os.Stdout
	stderrW = os.Stderr
)

func main() {
	flag.Parse()

	// Validation
	if *textFlag == "" && *envFlag == "" {
		fmt.Fprintln(stderrW, "Missing -text or -env option!")
		os.Exit(127)
	}

	args := flag.Args()
	if len(args) > 0 {
		fmt.Fprintln(stderrW, "Too many arguments!")
		os.Exit(127)
	}

	var finalFlag string
	var finalKind string

	if *textFlag != "" {
		finalFlag = *textFlag
		finalKind = "text"
	} else {
		finalFlag = *envFlag
		finalKind = "env"
	}

	// Flag gets printed as a page
	mux := http.NewServeMux()
	mux.HandleFunc("/", httpLog(stdoutW, withAppHeaders(httpEcho(finalFlag, finalKind))))

	// Health endpoint
	mux.HandleFunc("/health", withAppHeaders(httpHealth()))

	server := &http.Server{
		Addr:    *listenFlag,
		Handler: mux,
	}
	serverCh := make(chan struct{})
	go func() {
		log.Printf("[INFO] server is listening on %s\n", *listenFlag)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("[ERR] server exited with: %s", err)
		}
		close(serverCh)
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	// Wait for interrupt
	<-signalCh

	log.Printf("[INFO] received interrupt, shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("[ERR] failed to shutdown server: %s", err)
	}

	// If we got this far, it was an interrupt, so don't exit cleanly
	os.Exit(2)
}

func getEnvStrOrDefault(k, d string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}

	return d
}

func httpEcho(v, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch kind {
		case "text":
			fmt.Fprintln(w, v)
		case "env":
			resolvedV, ok := os.LookupEnv(v)
			if !ok {
				fmt.Fprintln(w, fmt.Sprintf("failed resolving env var '%s'", v))
			} else {
				fmt.Fprintln(w, resolvedV)
			}
		default:
			panic("something went wrong, not cool!")
		}
	}
}

func httpHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"status":"ok"}`)
	}
}

const (
	httpLogDateFormat string = "2006/01/02 15:04:05"
	httpLogFormat     string = "%v %s %s \"%s %s %s\" %d %d \"%s\" %v\n"
)

// withAppHeaders adds application headers such as X-App-Version and X-App-Name.
func withAppHeaders(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r)
	}
}

// metaResponseWriter is a response writer that saves information about the
// response for logging.
type metaResponseWriter struct {
	writer http.ResponseWriter
	status int
	length int
}

// Header implements the http.ResponseWriter interface.
func (w *metaResponseWriter) Header() http.Header {
	return w.writer.Header()
}

// WriteHeader implements the http.ResponseWriter interface.
func (w *metaResponseWriter) WriteHeader(s int) {
	w.status = s
	w.writer.WriteHeader(s)
}

// Write implements the http.ResponseWriter interface.
func (w *metaResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.length = len(b)
	return w.writer.Write(b)
}

// httpLog accepts an io object and logs the request and response objects to the
// given io.Writer.
func httpLog(out io.Writer, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mrw metaResponseWriter
		mrw.writer = w

		defer func(start time.Time) {
			status := mrw.status
			length := mrw.length
			end := time.Now()
			dur := end.Sub(start)
			fmt.Fprintf(out, httpLogFormat,
				end.Format(httpLogDateFormat),
				r.Host, r.RemoteAddr, r.Method, r.URL.Path, r.Proto,
				status, length, r.UserAgent(), dur)
		}(time.Now())

		h(&mrw, r)
	}
}
