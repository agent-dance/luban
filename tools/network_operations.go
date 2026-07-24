package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// HttpGetTool makes HTTP GET requests
type HttpGetTool struct{}

func (t *HttpGetTool) Name() string {
	return "HttpGet"
}

func (t *HttpGetTool) Description() string {
	return "Make an HTTP GET request"
}

func (t *HttpGetTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to fetch",
			},
			"timeout_seconds": map[string]any{
				"type":        "number",
				"description": "Request timeout in seconds (default: 30)",
			},
		},
		Required: []string{"url"},
	}
}

func (t *HttpGetTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	urlStr, err := MustGetStringField(input, "url")
	if err != nil {
		return ErrorResponse(err), nil
	}

	timeout := GetIntField(input, "timeout_seconds", 30)

	// Create context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlStr, nil)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkRequestCreateFailed, err), nil
	}

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkHTTPRequestFailed, err), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkResponseReadFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status":       resp.StatusCode,
		"url":          urlStr,
		"body":         string(body),
		"content_type": resp.Header.Get("Content-Type"),
	})
}

// HttpPostTool makes HTTP POST requests
type HttpPostTool struct{}

func (t *HttpPostTool) Name() string {
	return "HttpPost"
}

func (t *HttpPostTool) Description() string {
	return "Make an HTTP POST request"
}

func (t *HttpPostTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to post to",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Request body",
			},
			"content_type": map[string]any{
				"type":        "string",
				"description": "Content-Type header (default: application/json)",
			},
			"timeout_seconds": map[string]any{
				"type":        "number",
				"description": "Request timeout in seconds (default: 30)",
			},
		},
		Required: []string{"url", "body"},
	}
}

func (t *HttpPostTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	urlStr, err := MustGetStringField(input, "url")
	if err != nil {
		return ErrorResponse(err), nil
	}

	body, err := MustGetStringField(input, "body")
	if err != nil {
		return ErrorResponse(err), nil
	}

	contentType := GetStringField(input, "content_type", "application/json")
	timeout := GetIntField(input, "timeout_seconds", 30)

	// Create context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, urlStr, strings.NewReader(body))
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkRequestCreateFailed, err), nil
	}

	req.Header.Set("Content-Type", contentType)

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkHTTPRequestFailed, err), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkResponseReadFailed, err), nil
	}

	return ResponseJSON(map[string]any{
		"status":       resp.StatusCode,
		"url":          urlStr,
		"body":         string(respBody),
		"content_type": resp.Header.Get("Content-Type"),
	})
}

// DnsTool performs DNS lookups
type DnsTool struct{}

func (t *DnsTool) Name() string {
	return "Dns"
}

func (t *DnsTool) Description() string {
	return "Perform DNS lookups"
}

func (t *DnsTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"host": map[string]any{
				"type":        "string",
				"description": "Hostname to resolve",
			},
			"record_type": map[string]any{
				"type":        "string",
				"description": "DNS record type (A, AAAA, MX, etc. - default: A)",
			},
		},
		Required: []string{"host"},
	}
}

func (t *DnsTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	host, err := MustGetStringField(input, "host")
	if err != nil {
		return ErrorResponse(err), nil
	}

	recordType := GetStringField(input, "record_type", "A")

	resolver := &net.Resolver{}

	switch strings.ToUpper(recordType) {
	case "A":
		ips, err := resolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return toolRuntimeErrorf(i18n.KeyToolNetworkDNSLookupFailed, err), nil
		}
		var ipStrs []string
		for _, ip := range ips {
			ipStrs = append(ipStrs, ip.String())
		}
		return ResponseJSON(map[string]any{
			"host": host,
			"type": "A",
			"ips":  ipStrs,
		})

	case "AAAA":
		ips, err := resolver.LookupIP(ctx, "ip6", host)
		if err != nil {
			return toolRuntimeErrorf(i18n.KeyToolNetworkDNSLookupFailed, err), nil
		}
		var ipStrs []string
		for _, ip := range ips {
			ipStrs = append(ipStrs, ip.String())
		}
		return ResponseJSON(map[string]any{
			"host": host,
			"type": "AAAA",
			"ips":  ipStrs,
		})

	case "MX":
		mxs, err := resolver.LookupMX(ctx, host)
		if err != nil {
			return toolRuntimeErrorf(i18n.KeyToolNetworkDNSLookupFailed, err), nil
		}
		var mxStrs []string
		for _, mx := range mxs {
			mxStrs = append(mxStrs, fmt.Sprintf("%s (priority: %d)", mx.Host, mx.Pref))
		}
		return ResponseJSON(map[string]any{
			"host": host,
			"type": "MX",
			"mxs":  mxStrs,
		})

	case "NS":
		nss, err := resolver.LookupNS(ctx, host)
		if err != nil {
			return toolRuntimeErrorf(i18n.KeyToolNetworkDNSLookupFailed, err), nil
		}
		var nsStrs []string
		for _, ns := range nss {
			nsStrs = append(nsStrs, ns.Host)
		}
		return ResponseJSON(map[string]any{
			"host": host,
			"type": "NS",
			"nss":  nsStrs,
		})

	default:
		return toolRuntimeErrorf(i18n.KeyToolNetworkRecordUnsupported, recordType), nil
	}
}

// PingTool checks if a host is reachable
type PingTool struct{}

func (t *PingTool) Name() string {
	return "Ping"
}

func (t *PingTool) Description() string {
	return "Check if a host is reachable"
}

func (t *PingTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"host": map[string]any{
				"type":        "string",
				"description": "Host to ping",
			},
			"timeout_seconds": map[string]any{
				"type":        "number",
				"description": "Ping timeout in seconds (default: 5)",
			},
		},
		Required: []string{"host"},
	}
}

func (t *PingTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	host, err := MustGetStringField(input, "host")
	if err != nil {
		return ErrorResponse(err), nil
	}

	timeout := GetIntField(input, "timeout_seconds", 5)

	// Try to resolve the host
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkHostResolveFailed, err), nil
	}

	if len(ips) == 0 {
		return toolRuntimeErrorf(i18n.KeyToolNetworkNoIP, host), nil
	}

	// Try to dial the host on port 80
	addr := net.JoinHostPort(ips[0].String(), "80")
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeout)*time.Second)
	if err != nil {
		return ResponseJSON(map[string]any{
			"host":      host,
			"reachable": false,
			"error":     err.Error(),
		})
	}
	conn.Close()

	return ResponseJSON(map[string]any{
		"host":      host,
		"reachable": true,
		"ip":        ips[0].String(),
	})
}

// PortCheckTool checks if a port is open
type PortCheckTool struct{}

func (t *PortCheckTool) Name() string {
	return "PortCheck"
}

func (t *PortCheckTool) Description() string {
	return "Check if a port is open on a host"
}

func (t *PortCheckTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"host": map[string]any{
				"type":        "string",
				"description": "Hostname or IP address",
			},
			"port": map[string]any{
				"type":        "number",
				"description": "Port number to check",
			},
			"timeout_seconds": map[string]any{
				"type":        "number",
				"description": "Connection timeout in seconds (default: 5)",
			},
		},
		Required: []string{"host", "port"},
	}
}

func (t *PortCheckTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	host, err := MustGetStringField(input, "host")
	if err != nil {
		return ErrorResponse(err), nil
	}

	port := GetIntField(input, "port", 0)
	if port <= 0 || port > 65535 {
		return toolRuntimeErrorf(i18n.KeyToolNetworkPortInvalid, port), nil
	}

	timeout := GetIntField(input, "timeout_seconds", 5)

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeout)*time.Second)
	if err != nil {
		return ResponseJSON(map[string]any{
			"host":  host,
			"port":  port,
			"open":  false,
			"error": err.Error(),
		})
	}
	conn.Close()

	return ResponseJSON(map[string]any{
		"host": host,
		"port": port,
		"open": true,
	})
}

// IpaddressTool gets the local IP address
type IpaddressTool struct{}

func (t *IpaddressTool) Name() string {
	return "Ipaddress"
}

func (t *IpaddressTool) Description() string {
	return "Get local IP address"
}

func (t *IpaddressTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *IpaddressTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkLocalIPFailed, err), nil
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return ResponseJSON(map[string]string{
		"ipaddress": localAddr.IP.String(),
	})
}

// UrlParseTool parses URLs
type UrlParseTool struct{}

func (t *UrlParseTool) Name() string {
	return "UrlParse"
}

func (t *UrlParseTool) Description() string {
	return "Parse and extract components from a URL"
}

func (t *UrlParseTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to parse",
			},
		},
		Required: []string{"url"},
	}
}

func (t *UrlParseTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	urlStr, err := MustGetStringField(input, "url")
	if err != nil {
		return ErrorResponse(err), nil
	}

	parsedUrl, err := url.Parse(urlStr)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkURLParseFailed, err), nil
	}

	username := ""
	if parsedUrl.User != nil {
		username = parsedUrl.User.Username()
	}

	return ResponseJSON(map[string]any{
		"scheme":   parsedUrl.Scheme,
		"host":     parsedUrl.Host,
		"hostname": parsedUrl.Hostname(),
		"port":     parsedUrl.Port(),
		"path":     parsedUrl.Path,
		"query":    parsedUrl.RawQuery,
		"fragment": parsedUrl.Fragment,
		"user":     username,
	})
}

// WhoisTool performs WHOIS lookups
type WhoisTool struct{}

func (t *WhoisTool) Name() string {
	return "Whois"
}

func (t *WhoisTool) Description() string {
	return "Perform WHOIS lookup for a domain"
}

func (t *WhoisTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"domain": map[string]any{
				"type":        "string",
				"description": "Domain name to look up",
			},
			"timeout_seconds": map[string]any{
				"type":        "number",
				"description": "Lookup timeout in seconds (default: 10)",
			},
		},
		Required: []string{"domain"},
	}
}

func (t *WhoisTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	domain, err := MustGetStringField(input, "domain")
	if err != nil {
		return ErrorResponse(err), nil
	}

	timeout := GetIntField(input, "timeout_seconds", 10)

	// Connect to WHOIS server with timeout
	conn, err := net.DialTimeout("tcp", "whois.iana.org:43", time.Duration(timeout)*time.Second)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkWHOISConnectFailed, err), nil
	}
	defer conn.Close()

	// Set read/write deadline
	conn.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))

	// Send query
	_, err = fmt.Fprintf(conn, "%s\r\n", domain)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkWHOISSendFailed, err), nil
	}

	// Read response
	respBytes, err := io.ReadAll(conn)
	if err != nil {
		return toolRuntimeErrorf(i18n.KeyToolNetworkWHOISReadFailed, err), nil
	}

	return ResponseJSON(map[string]string{
		"domain": domain,
		"whois":  string(respBytes),
	})
}
