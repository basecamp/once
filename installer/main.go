package main

import (
	"embed"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/distribution/reference"
)

//go:embed templates/*
var templateFS embed.FS

type InstallScriptArgs struct {
	ImageRef string
}

var installImageAliases = map[string]string{
	"screenote": "ghcr.io/ivankuznetsov/screenote",
}

func withLogging(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rl := &responseLogger{ResponseWriter: w, status: http.StatusOK}

		h.ServeHTTP(rl, r)

		duration := time.Since(start)
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
				clientIP = host
			} else {
				clientIP = r.RemoteAddr
			}
		} else {
			clientIP = strings.Split(clientIP, ",")[0]
			clientIP = strings.TrimSpace(clientIP)
		}

		slog.Info("request",
			"ip", clientIP,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rl.status,
			"size", rl.size,
			"duration", duration,
		)
	}
}

type responseLogger struct {
	http.ResponseWriter
	status int
	size   int
}

func (rl *responseLogger) WriteHeader(status int) {
	rl.status = status
	rl.ResponseWriter.WriteHeader(status)
}

func (rl *responseLogger) Write(b []byte) (int, error) {
	n, err := rl.ResponseWriter.Write(b)
	rl.size += n
	return n, err
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func newInstallScriptHandler(template *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		imageRef := expandInstallImageAlias(r.PathValue("image"))
		if imageRef != "" {
			if _, err := reference.ParseNormalizedNamed(imageRef); err != nil {
				http.Error(w, "invalid image reference", http.StatusBadRequest)
				return
			}
		}

		args := InstallScriptArgs{
			ImageRef: imageRef,
		}

		err := template.ExecuteTemplate(w, "install.sh", args)
		if err != nil {
			slog.Error("Failed to execute template", "error", err)
		}
	}
}

func expandInstallImageAlias(imageRef string) string {
	if expanded, ok := installImageAliases[imageRef]; ok {
		return expanded
	}

	return imageRef
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	template, err := template.ParseFS(templateFS, "templates/*")
	if err != nil {
		panic(err)
	}

	http.HandleFunc("GET /up", healthHandler)
	http.HandleFunc("GET /{image...}", withLogging(newInstallScriptHandler(template)))

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "80"
	}

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
