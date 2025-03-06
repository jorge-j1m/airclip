package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Global logger
var logger *log.Logger

// To test: curl -s -X POST -H "Authorization: Bearer local-use-only" -d "testfromcurl" http://localhost:9123/notify

const (
	// exitCodeErr is the code to return in case of any error in the program
	exitCodeErr = 1
	// exitCodeInterrupt is the code to return in case of an interrupt signal
	exitCodeInterrupt = 2
)

var (
	// Default to port 9123, which is likely not used by other common services
	port       = flag.String("port", "9123", "HTTP port to listen on")
	listenAddr = flag.String("listen", "0.0.0.0", "IP address to listen on (use local IPs only)")
	authToken  = flag.String("token", "local-use-only", "Simple authentication token")
	localOnly  = flag.Bool("local-only", true, "Restrict to local network connections only")
	allowCORS  = flag.Bool("cors", true, "Enable CORS for cross-origin requests")
	logDir     = flag.String("logdir", "/tmp", "Directory to store log files")
)

func main() {
	flag.Parse()

	// Create context with cancellation
	ctx, cancel := context.WithCancelCause(context.Background())

	// Setup signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer func() {
		signal.Stop(signalChan)
		cancel(nil)
	}()

	// Setup logging with timestamp in filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFilePath := filepath.Join(*logDir, fmt.Sprintf("notification-server_%s.log", timestamp))
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(exitCodeErr)
	}
	defer logFile.Close()

	// Set up global logger
	logger = log.New(logFile, "", log.LstdFlags|log.Lshortfile)

	// Log and print startup message
	logf("Starting notification server (log file: %s)\n", logFilePath)

	// Display warning if not in local-only mode
	if !*localOnly {
		logf("WARNING: Running with local-only protection disabled. This is not recommended.\n")
	}

	// Print local IP addresses for user reference
	printLocalIPs()

	// Handle signals in a goroutine
	go func() {
		select {
		case <-signalChan: // first signal, cancel context
			logf("Interrupt signal received, stopping gracefully\n")
			cancel(errors.New("interrupt signal received"))
		case <-ctx.Done():
		}
		<-signalChan // second signal, hard exit
		logf("Interrupt signal received again, stopping immediately\n")
		os.Exit(exitCodeInterrupt)
	}()

	// Run the server
	if err := run(ctx, cancel); err != nil {
		logf("Error running the server: %v\n", err)
		os.Exit(exitCodeErr)
	}

	logf("Server shutdown completed\n")
}

func run(ctx context.Context, cancel context.CancelCauseFunc) error {
	// Set up HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/notify", handleNotification)
	mux.HandleFunc("/health", handleHealthCheck)

	// Create server with reasonable timeouts
	addr := fmt.Sprintf("%s:%s", *listenAddr, *port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Use WaitGroup to coordinate shutdown
	var wg sync.WaitGroup

	// Start server in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		logf("Starting notification server on %s (LOCAL NETWORK USE ONLY)\n", addr)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("HTTP server failed: %v\n", err)
			cancel(err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	logf("Context done: %v\n", context.Cause(ctx))

	// Create a timeout context for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	// Attempt graceful shutdown
	logf("Shutting down server gracefully...\n")
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	// Wait for server goroutine to finish
	wg.Wait()
	return nil
}

// Log and print a message to the console
func logf(format string, v ...any) {
	logger.Printf(format, v...)
	fmt.Printf(format, v...)
}

// handleNotification processes notification requests
func handleNotification(w http.ResponseWriter, r *http.Request) {
	// Check if request is from local network if local-only is enabled
	if *localOnly {
		clientIP := getClientIP(r)
		if !isLocalIP(clientIP) {
			logf("Rejected non-local request from %s\n", clientIP)
			http.Error(w, "This service is restricted to local network use only", http.StatusForbidden)
			return
		}
	}

	// Enable CORS if requested
	if *allowCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Only accept POST requests
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check authentication if a token is set
	if *authToken != "" {
		providedToken := r.Header.Get("Authorization")
		if providedToken != "Bearer "+*authToken {
			logf("Unauthorized access attempt from %s\n", getClientIP(r))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Read the message from the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logf("Error reading request body: %v\n", err)
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	message := string(body)
	if message == "" {
		http.Error(w, "Empty message", http.StatusBadRequest)
		return
	}

	logf("Received notification request from %s: %d chars\n", getClientIP(r), len(message))

	// Send notification and copy to clipboard
	if err := processNotification(message); err != nil {
		logf("Error processing notification: %v\n", err)
		http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Notification sent and text copied to clipboard\n"))
}

// handleHealthCheck provides a simple health check endpoint
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	clientIP := getClientIP(r)
	logf("Health check from %s\n", clientIP)

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Notification server is running\n"))
}

// processNotification sends a desktop notification and copies text to clipboard
func processNotification(message string) error {
	// First copy to clipboard using xclip
	clipCmd := exec.Command("bash", "-c", fmt.Sprintf("echo -n %q | xclip -selection clipboard", message))
	if err := clipCmd.Run(); err != nil {
		return fmt.Errorf("failed to copy to clipboard: %v", err)
	}

	// Then send notification
	notifyCmd := exec.Command("notify-send",
		"--app-name=NotificationServer",
		"--icon=dialog-information",
		"Text copied to clipboard from Airclip",
	)

	return notifyCmd.Run()
}

// Helper functions for local network validation

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check for X-Forwarded-For header first (in case behind proxy)
	forwardedIP := r.Header.Get("X-Forwarded-For")
	if forwardedIP != "" {
		// X-Forwarded-For can contain multiple IPs; we want the client IP
		ips := strings.Split(forwardedIP, ",")
		return strings.TrimSpace(ips[0])
	}

	// Otherwise extract from RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr // Return as is if can't split
	}
	return ip
}

// isLocalIP checks if an IP address is from the local network
func isLocalIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Check for loopback addresses (127.0.0.0/8)
	if ip.IsLoopback() {
		return true
	}

	// Check for private network ranges
	// 10.0.0.0/8
	if ip[0] == 10 {
		return true
	}

	// 172.16.0.0/12
	if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return true
	}

	// 192.168.0.0/16
	if ip[0] == 192 && ip[1] == 168 {
		return true
	}

	// fe80::/10 (Link-local IPv6)
	if ip.IsLinkLocalUnicast() {
		return true
	}

	return false
}

// printLocalIPs prints all local IP addresses for the user to configure their iOS device
func printLocalIPs() {
	interfaces, err := net.Interfaces()
	if err != nil {
		logf("Failed to get network interfaces: %v\n", err)
		return
	}

	logf("Available local IP addresses to use in your iOS shortcut:\n")
	logf("----------------------------------------------------------\n")

	for _, iface := range interfaces {
		// Skip loopback and inactive interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Skip loopback and non-IPv4 addresses for simplicity
			if ip.IsLoopback() || ip.To4() == nil {
				continue
			}

			logf("Interface: %-10s  IP: %-15s  URL: http://%s:%s/notify\n",
				iface.Name, ip.String(), ip.String(), *port)
		}
	}
	logf("----------------------------------------------------------\n")
	logf("USE ONE OF THESE IPs IN YOUR iOS SHORTCUT\n\n")
}
