package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/dreikanter/notes-view/internal/logging"
	"github.com/dreikanter/notes-view/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve markdown notes with live preview",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().IntP("port", "p", 0, "port to listen on (default: $NOTESVIEW_PORT, auto)")
	serveCmd.Flags().BoolP("open", "o", false, "open browser on start")
	serveCmd.Flags().String("editor", "", "editor command (default: $NOTESVIEW_EDITOR, $VISUAL, $EDITOR)")
	serveCmd.Flags().String("path", "", "notes root path or file (default: $NOTESVIEW_PATH, $NOTES_PATH, .)")
	serveCmd.Flags().String("log-level", "", "log level: debug, info, warn, error (default: $NOTESVIEW_LOG_LEVEL, info)")
	serveCmd.Flags().String("log-format", "", "log output format: text or json (default: $NOTESVIEW_LOG_FORMAT, text)")
	serveCmd.Flags().String("log-file", "", "additional log sink file path (default: $NOTESVIEW_LOG_FILE)")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	if port == 0 && !cmd.Flags().Changed("port") {
		if v := os.Getenv("NOTESVIEW_PORT"); v != "" {
			p, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("invalid NOTESVIEW_PORT %q: %w", v, err)
			}
			if p < 0 || p > 65535 {
				return fmt.Errorf("NOTESVIEW_PORT %d out of range 0..65535", p)
			}
			port = p
		}
	}
	open, _ := cmd.Flags().GetBool("open")
	editor, _ := cmd.Flags().GetString("editor")
	path, _ := cmd.Flags().GetString("path")
	logLevel, _ := cmd.Flags().GetString("log-level")
	logFormat, _ := cmd.Flags().GetString("log-format")
	logFile, _ := cmd.Flags().GetString("log-file")

	if editor == "" {
		for _, env := range []string{"NOTESVIEW_EDITOR", "VISUAL", "EDITOR"} {
			if v := os.Getenv(env); v != "" {
				editor = v
				break
			}
		}
	}

	if path == "" {
		for _, env := range []string{"NOTESVIEW_PATH", "NOTES_PATH"} {
			if v := os.Getenv(env); v != "" {
				path = v
				break
			}
		}
	}
	if path == "" {
		path = "."
	}
	path = expandTilde(path)

	if logLevel == "" {
		logLevel = os.Getenv("NOTESVIEW_LOG_LEVEL")
	}
	if logFormat == "" {
		logFormat = os.Getenv("NOTESVIEW_LOG_FORMAT")
	}
	if logFile == "" {
		logFile = os.Getenv("NOTESVIEW_LOG_FILE")
	}

	logger, logCloser, err := logging.New(logging.Config{
		Level:  logLevel,
		Format: logFormat,
		File:   logFile,
	})
	if err != nil {
		return fmt.Errorf("configure logger: %w", err)
	}
	if logCloser != nil {
		defer func() { _ = logCloser.Close() }()
	}

	root, initialFile, err := resolvePath(path)
	if err != nil {
		return err
	}

	srv, err := server.NewServer(root, editor, logger)
	if err != nil {
		return err
	}
	if err := srv.StartWatcher(); err != nil {
		logger.Warn("file watcher failed to start", "err", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}

	baseURL := "http://" + listener.Addr().String()
	logger.Info("notesview serving", "root", root, "url", baseURL)

	if open {
		target := baseURL
		if initialFile != "" {
			target = baseURL + "/view/" + initialFile
		}
		openBrowser(target)
	}

	httpServer := &http.Server{Handler: srv.Routes()}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down")
		srv.Shutdown()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			logger.Warn("http server shutdown error", "err", err)
		}
	}()

	if err := httpServer.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func resolvePath(p string) (root, initialFile string, err error) {
	info, err := os.Stat(p)
	if err != nil {
		return "", "", err
	}
	var dir string
	if info.IsDir() {
		dir = p
	} else {
		dir = filepath.Dir(p)
		initialFile = filepath.Base(p)
	}
	root, err = filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	return root, initialFile, nil
}

func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home + p[1:]
	}
	return p
}

func openBrowser(url string) {
	cmd := browserCommand(runtime.GOOS, url)
	if cmd == nil {
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to open browser: %v\n", err)
		return
	}
	go cmd.Wait()
}

func browserCommand(goos, url string) *exec.Cmd {
	switch goos {
	case "darwin":
		return exec.Command("open", url)
	case "linux":
		return exec.Command("xdg-open", url)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return nil
	}
}
