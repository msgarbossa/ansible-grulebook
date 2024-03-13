package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	httpIp = "127.0.0.1"
)

var httpPort int

func tcpGather(ip string, ports []string) map[string]string {
	// check emqx 1883, 8083 port

	results := make(map[string]string)
	for _, port := range ports {
		address := net.JoinHostPort(ip, port)
		// 3 second timeout
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err != nil {
			results[port] = "failed"
			// todo log handler
		} else {
			if conn != nil {
				results[port] = "success"
				_ = conn.Close()
			} else {
				results[port] = "failed"
			}
		}
	}
	return results
}

// Only exported functions (first letter capital) are run with "go test"
func TestHttpListener(t *testing.T) {

	// default log level to LevelWarn (prints WARN and ERROR)
	slog.SetLogLoggerLevel(slog.LevelInfo)

	var (
		ctx, cancel = context.WithCancel(context.Background())
	)

	defer func() {
		// Shutdown. Cancel application context will kill all attached tasks.
		fmt.Println("Running cancel()")
		cancel()
	}()

	config_obj, err := readConf("config.yml")
	if err != nil {
		panic(err)
	}

	// Prepare knowledgebase library and load it with our rule.
	knowledgeBase = getRules()

	// TODO: this should unmarshal JSON instead of looping
	for _, source_obj := range config_obj.Sources {
		// httpAddr = fmt.Sprintf(":%d", source_obj.Port)
		httpPort = source_obj.Port
	}
	httpAddr = fmt.Sprintf("%s:%d", httpIp, httpPort)

	go startHttpListener(ctx)
	time.Sleep(time.Second * 1)

	httpPortStr := fmt.Sprintf("%d", httpPort)

	httpPorts := []string{httpPortStr}

	results := tcpGather(httpIp, httpPorts)
	for port, status := range results {
		slog.Info(fmt.Sprintf("port test: %s status: %s\n", port, status))
	}
	if results[httpPortStr] != "success" {
		t.Errorf("expected port %d == success but got %s", httpPort, results[httpPortStr])
	}
}

func TestDataIngestion(t *testing.T) {

	// slurp entire file contents into memory
	contents, err := os.ReadFile("./tests/alertmanager-webhook.json")
	if err != nil {
		t.Error("failed to read JSON input")
	}

	requestURL := fmt.Sprintf("http://localhost:%d/alerts", httpPort)

	// convert contents to bytes
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write([]byte(contents)); err != nil {
		t.Errorf("Expected webhook test body to be valid: %s", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal((err))
	}

	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewBuffer(b.Bytes()))
	if err != nil {
		fmt.Printf("client: could not create webhook test request: %s\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	client := http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("Expected HTTP POST for webhook test to be successful: %s", err)
	}
	t.Log(resp)

	// need to close response body even if you don't want to read it.
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Println("Non-OK HTTP status:", resp.StatusCode)
	}

	// TODO: verify results/parsing

	// clear the buffer before additional requests (or else the previous entry still exists)
	// b.Reset()

}

func TestConcurrency(t *testing.T) {

	slog.SetLogLoggerLevel(slog.LevelWarn)

	// slurp entire file contents into memory
	contents, err := os.ReadFile("./tests/alertmanager-webhook.json")
	if err != nil {
		t.Error("failed to read JSON input")
	}

	requestURL := fmt.Sprintf("http://localhost:%d/alerts", httpPort)

	// convert contents to bytes
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write([]byte(contents)); err != nil {
		t.Errorf("Expected webhook test body to be valid: %s", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal((err))
	}

	req, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewBuffer(b.Bytes()))
	if err != nil {
		fmt.Printf("client: could not create webhook test request: %s\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	client := http.Client{
		Timeout: 10 * time.Second,
	}

	for i := 1; i <= 100; i++ {

		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("Expected HTTP POST for webhook test to be successful: %s", err)
		}
		t.Log(resp)

		// need to close response body even if you don't want to read it.
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Println("Non-OK HTTP status:", resp.StatusCode)
		}

		// TODO: verify results/parsing

		// clear the buffer before additional requests (or else the previous entry still exists)
		// b.Reset()

	}

}
