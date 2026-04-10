package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/recolabs/gnata"
)

const (
	exitOK = iota
	exitFlagError
	exitRequestParseError
	exitRequestError
	exitResponseError
	exitJSONataError
)

type config struct {
	rawFile  string
	method   string
	url      string
	headers  http.Header
	data     string
	dataFile string
	jsonata  string
	timeout  time.Duration
	pretty   bool
	useRaw   bool
}

func main() {
	cfg := parseFlags()

	var rawRequest string
	if cfg.useRaw {
		raw, err := readRawRequest(cfg.rawFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading raw request: %v\n", err)
			os.Exit(exitRequestParseError)
		}
		rawRequest = raw
	}

	client := &http.Client{
		Timeout: cfg.timeout,
	}

	var req *http.Request
	var err error

	if cfg.useRaw {
		req, err = buildRequestFromRaw(rawRequest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing raw request: %v\n", err)
			os.Exit(exitRequestParseError)
		}
		// ensure context with timeout
		req = req.WithContext(context.Background())
	} else {
		body, err := buildRequestBody(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(exitRequestError)
		}
		req, err = http.NewRequestWithContext(context.Background(), cfg.method, cfg.url, body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error building request: %v\n", err)
			os.Exit(exitRequestError)
		}
		req.Header = cfg.headers
		if body != nil && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error performing request: %v\n", err)
		os.Exit(exitRequestError)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading response body: %v\n", err)
		os.Exit(exitResponseError)
	}

	// always print status line to stderr
	fmt.Fprintf(os.Stderr, "HTTP %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))

	// prepare output: apply JSONata if needed, then maybe pretty-print
	output := respBody
	if cfg.jsonata != "" {
		transformed, err := applyJSONata(respBody, cfg.jsonata)
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSONata error: %v\n", err)
			os.Exit(exitJSONataError)
		}
		output = transformed
	}

	if cfg.pretty {
		prettyOut, err := prettyPrintJSON(output)
		if err == nil {
			output = prettyOut
		}
		// if not valid JSON, keep original output (do nothing)
	}

	fmt.Println(string(output))
}

func parseFlags() config {
	var (
		method   string
		urlStr   string
		headers  headerSlice
		data     string
		dataFile string
		rawFile  string
		jsonata  string
		timeout  int
		pretty   bool
	)

	flag.StringVar(&method, "method", "GET", "HTTP method (used when -raw is not set)")
	flag.StringVar(&urlStr, "url", "", "Request URL (used when -raw is not set)")
	flag.Var(&headers, "H", "Header 'Key: Value' (can be repeated, used with -method/-url)")
	flag.StringVar(&data, "data", "", "Request body string (used with -method/-url)")
	flag.StringVar(&dataFile, "data-file", "", "File with request body (overrides -data)")
	flag.StringVar(&rawFile, "raw", "", "File containing raw HTTP request (RFC 2616). If not set but no other flags given, reads from stdin.")
	flag.StringVar(&jsonata, "jsonata", "", "JSONata expression to transform response")
	flag.IntVar(&timeout, "timeout", 30, "Request timeout in seconds")
	flag.BoolVar(&pretty, "pretty", false, "Pretty-print JSON response (indented)")
	flag.BoolVar(&pretty, "p", false, "Shorthand for -pretty")

	flag.Parse()

	cfg := config{
		method:   strings.ToUpper(method),
		url:      urlStr,
		headers:  http.Header(headers),
		data:     data,
		dataFile: dataFile,
		rawFile:  rawFile,
		jsonata:  jsonata,
		timeout:  time.Duration(timeout) * time.Second,
		pretty:   pretty,
		useRaw:   rawFile != "" || (rawFile == "" && flag.NFlag() == 0 && flag.NArg() == 0),
	}

	// If -raw is not set and no flags were provided, assume raw from stdin
	if !cfg.useRaw && rawFile == "" && flag.NFlag() == 0 && flag.NArg() == 0 {
		cfg.useRaw = true
		cfg.rawFile = "" // stdin
	}

	return cfg
}

func readRawRequest(filename string) (string, error) {
	var r io.Reader
	if filename == "" {
		r = os.Stdin
	} else {
		f, err := os.Open(filename)
		if err != nil {
			return "", err
		}
		defer f.Close()
		r = f
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildRequestFromRaw(raw string) (*http.Request, error) {
	reader := bufio.NewReader(strings.NewReader(raw))
	req, err := http.ReadRequest(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse raw request: %w", err)
	}
	defer req.Body.Close()

	// http.ReadRequest expects Request-URI to be a path, but we allow absolute URL
	// because many people write "POST https://example.com/foo HTTP/1.1"
	var requestURL *url.URL
	if strings.HasPrefix(req.RequestURI, "http://") || strings.HasPrefix(req.RequestURI, "https://") {
		requestURL, err = url.Parse(req.RequestURI)
		if err != nil {
			return nil, fmt.Errorf("invalid absolute URL in request line: %w", err)
		}
	} else {
		// Build URL from Host header and RequestURI
		scheme := "http"
		if req.TLS != nil {
			scheme = "https"
		}
		// Try to infer scheme from Host (not reliable, but typical)
		host := req.Host
		if host == "" {
			host = req.Header.Get("Host")
		}
		if strings.Contains(host, ":") {
			// keep as is
		}
		requestURL = &url.URL{
			Scheme: scheme,
			Host:   host,
			Path:   req.RequestURI,
		}
	}

	// Replace the URL in the request
	req.URL = requestURL
	req.Host = requestURL.Host

	// Re-read body (since ReadRequest consumed it)
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Remove the 'Host' header from Header map because it should be managed separately
	req.Header.Del("Host")

	return req, nil
}

type headerSlice http.Header

func (h *headerSlice) String() string { return fmt.Sprintf("%v", http.Header(*h)) }
func (h *headerSlice) Set(value string) error {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid header format, expected 'Key: Value', got %q", value)
	}
	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	if key == "" {
		return fmt.Errorf("empty header key")
	}
	(*h)[key] = append((*h)[key], val)
	return nil
}

func buildRequestBody(cfg config) (io.Reader, error) {
	if cfg.dataFile != "" {
		content, err := os.ReadFile(cfg.dataFile)
		if err != nil {
			return nil, fmt.Errorf("reading data file: %w", err)
		}
		return bytes.NewReader(content), nil
	}
	if cfg.data != "" {
		return strings.NewReader(cfg.data), nil
	}
	return nil, nil
}

func applyJSONata(body []byte, exprStr string) ([]byte, error) {
	rawJSON := json.RawMessage(body)

	expr, err := gnata.Compile(exprStr)
	if err != nil {
		return nil, fmt.Errorf("compiling JSONata expression: %w", err)
	}

	result, err := expr.EvalBytes(context.Background(), rawJSON)
	if err != nil {
		return nil, fmt.Errorf("evaluating JSONata: %w", err)
	}

	out, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling JSONata result: %w", err)
	}
	return out, nil
}

func prettyPrintJSON(body []byte) ([]byte, error) {
	var out bytes.Buffer
	err := json.Indent(&out, body, "", "  ")
	if err != nil {
		return body, err
	}
	return out.Bytes(), nil
}
