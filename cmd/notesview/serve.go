package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/dreikanter/notesview/internal/logging"
	"github.com/dreikanter/notesview/internal/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve markdown notes with live preview",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().IntP("port", "p", 0, "port to listen on (default: auto)")
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

	return http.Serve(listener, srv.Routes())
}

func resolvePath(p string) (root, initialFile string, err error) {
	info, err := os.Stat(p)
	if err != nil {
		return "", "", err
	}
	if info.IsDir() {
		return p, "", nil
	}
	return filepath.Dir(p), filepath.Base(p), nil
}

func expandTilde(p string) string {
	if len(p) > 0 && p[0] == '~' {
		home, _ := os.UserHomeDir()
		return home + p[1:]
	}
	return p
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	cmd.Start()
}
