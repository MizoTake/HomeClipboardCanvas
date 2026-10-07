package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"homeclipboardcanvas/internal/config"
	"homeclipboardcanvas/internal/server"
	"homeclipboardcanvas/internal/startup"
	"homeclipboardcanvas/web"
)

var version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		args = []string{"serve"}
	}
	switch args[0] {
	case "version":
		fmt.Println("HomeClipboardCanvas", version)
		return nil
	case "help", "--help", "-h":
		fmt.Println("HomeClipboardCanvas serve|version|status|install-startup|uninstall-startup\nserve: --config path --bind IP --port 8443 --cert certificate.pem --key private-key.pem --dev-http\nLAN access needs no authentication. Startup commands are for the actual server PC only.")
		return nil
	case "serve", "status", "install-startup", "uninstall-startup":
	default:
		return errors.New("unknown command; use help")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	path := flags.String("config", filepath.Join(filepath.Dir(exe), "config.json"), "portable configuration")
	dev := flags.Bool("dev-http", false, "localhost HTTP development only")
	bind := flags.String("bind", "", "bind IP")
	port := flags.Int("port", 0, "listen port")
	cert := flags.String("cert", "", "TLS certificate PEM")
	key := flags.String("key", "", "TLS private key PEM")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	abs, err := filepath.Abs(*path)
	if err != nil {
		return err
	}
	c, err := config.Load(abs)
	if err != nil && !os.IsNotExist(err) {
		return errors.New("unable to read configuration")
	}
	if os.IsNotExist(err) {
		c = config.Default()
	}
	c.DevHTTP = *dev
	if *dev {
		c.Bind, c.Port = "127.0.0.1", 8080
	}
	if *bind != "" {
		c.Bind = *bind
	}
	if *port != 0 {
		c.Port = *port
	}
	if *cert != "" {
		c.Certificate = *cert
	}
	if *key != "" {
		c.PrivateKey = *key
	}
	if args[0] != "serve" {
		manager, err := startup.Current(exe, abs)
		if err != nil {
			return err
		}
		switch args[0] {
		case "status":
			validation := "ready"
			if c.Validate() != nil {
				validation = "incomplete; check certificates and limits"
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"version": version, "configExists": fileExists(abs), "configuration": validation, "startupRegistered": manager.Installed(), "bind": c.Bind, "port": c.Port})
		case "install-startup":
			if *dev {
				return errors.New("development cannot register startup")
			}
			if !fileExists(abs) {
				return errors.New("startup requires an explicit configuration file")
			}
			persisted, err := config.Load(abs)
			if err != nil {
				return errors.New("startup requires a valid portable configuration")
			}
			if err := persisted.Validate(); err != nil {
				return err
			}
			if err := manager.Install(); err != nil {
				return err
			}
			fmt.Println("Startup shortcut registered.")
			return nil
		case "uninstall-startup":
			if err := manager.Uninstall(); err != nil {
				return err
			}
			fmt.Println("Startup shortcut removed.")
			return nil
		}
	}
	if err := c.Validate(); err != nil {
		return err
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(c.LogLevel))); err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	networks, _ := c.Networks()
	app := server.New(server.Options{DevHTTP: c.DevHTTP, Limits: c.Limits(), AllowedNetworks: networks, Assets: web.Assets(), Logger: logger})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go app.ExpireLoop(ctx)
	httpServer := &http.Server{Addr: net.JoinHostPort(c.Bind, strconv.Itoa(c.Port)), Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn)}
	go func() {
		<-ctx.Done()
		timeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(timeout)
	}()
	logger.Info("server starting", "address", httpServer.Addr, "https", !c.DevHTTP, "ttlMinutes", c.TTLMinutes)
	if c.DevHTTP {
		err = httpServer.ListenAndServe()
	} else {
		err = httpServer.ListenAndServeTLS(c.Certificate, c.PrivateKey)
	}
	app.Store.Clear()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
