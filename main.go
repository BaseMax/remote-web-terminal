package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/basemax/remote-web-terminal/internal/auth"
	"github.com/basemax/remote-web-terminal/internal/config"
	"github.com/basemax/remote-web-terminal/internal/terminal"
)

func main() {
	cfgPath := flag.String("config", "config.json", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	sessions := auth.NewSessionStore(30 * time.Minute)
	terms := terminal.NewManager()

	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static"))))

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		auth.LoginHandler(w, r, cfg, sessions)
	})
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		auth.LogoutHandler(w, r, sessions)
	})

	protected := auth.Middleware(sessions)

	mux.Handle("/", protected(http.HandlerFunc(indexHandler)))
	mux.Handle("/api/terminal/create", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terminal.CreateHandler(w, r, terms, cfg)
	})))
	mux.Handle("/api/terminal/input", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terminal.InputHandler(w, r, terms)
	})))
	mux.Handle("/api/terminal/output", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terminal.OutputHandler(w, r, terms)
	})))
	mux.Handle("/api/terminal/resize", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terminal.ResizeHandler(w, r, terms)
	})))
	mux.Handle("/api/terminal/close", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terminal.CloseHandler(w, r, terms)
	})))
	mux.Handle("/api/terminal/heartbeat", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terminal.HeartbeatHandler(w, r, terms)
	})))

	addr := cfg.ListenAddr
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("remote-web-terminal listening on %s", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 70 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
	_ = os.Stderr
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "./web/index.html")
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
