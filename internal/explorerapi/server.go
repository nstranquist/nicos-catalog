package explorerapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

type Ready struct {
	URL     string
	Receipt explorercontract.ServeReceipt
}

type ServerConfig struct {
	Listen         string
	ProductVersion string
	Service        *Service
	Web            http.Handler
	OnReady        func(Ready) error
}

// ValidateListenAddress rejects every non-loopback address before Listen.
func ValidateListenAddress(ctx context.Context, address string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || host == "" {
		return fmt.Errorf("listen address must be LOOPBACK:PORT")
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 0 || value > 65535 {
		return fmt.Errorf("listen port is invalid")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !ip.IsLoopback() {
			return fmt.Errorf("listen address must be loopback")
		}
		return nil
	}
	if !strings.EqualFold(host, "localhost") {
		return fmt.Errorf("listen address must be loopback")
	}
	resolveCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	resolved, err := net.DefaultResolver.LookupIPAddr(resolveCtx, "localhost")
	if err != nil || len(resolved) == 0 {
		return fmt.Errorf("localhost did not resolve to loopback")
	}
	for _, item := range resolved {
		if !item.IP.IsLoopback() {
			return fmt.Errorf("localhost did not resolve only to loopback")
		}
	}
	return nil
}

// RunServer serves until ctx is canceled and then drains active reads.
func RunServer(ctx context.Context, config ServerConfig) error {
	if config.Listen == "" {
		config.Listen = "127.0.0.1:0"
	}
	if err := ValidateListenAddress(ctx, config.Listen); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	hosts, err := AllowedHostsForListener(listener.Addr().String())
	if err != nil {
		return err
	}
	h, err := NewHandler(config.Service, HandlerConfig{ProductVersion: config.ProductVersion, AllowedHosts: hosts, Web: config.Web})
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	ready := Ready{URL: "http://" + listener.Addr().String(), Receipt: explorercontract.ServeReceipt{URL: "http://" + listener.Addr().String(), EntityCount: len(config.Service.dataset.Entities)}}
	if config.OnReady != nil {
		if err := config.OnReady(ready); err != nil {
			return err
		}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-done:
		}
	}()
	err = server.Serve(listener)
	close(done)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
