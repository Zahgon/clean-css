package reader

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	protocolPattern = regexp.MustCompile(`(?i)^[a-z0-9.+-]+:`)
	portPattern     = regexp.MustCompile(`:[0-9]*$`)
)

// urlHost returns the host (hostname and port) the legacy url.parse of node
// finds in a URL, or the empty string when it finds none.
func urlHost(raw string) string {
	rest := strings.TrimSpace(raw)
	if q := strings.IndexAny(rest, "?#"); q >= 0 {
		rest = strings.ReplaceAll(rest[:q], `\`, "/") + rest[q:]
	} else {
		rest = strings.ReplaceAll(rest, `\`, "/")
	}

	protocol := protocolPattern.FindString(rest)
	if protocol != "" {
		rest = rest[len(protocol):]
	}
	if !strings.HasPrefix(rest, "//") || strings.EqualFold(protocol, "javascript:") {
		return ""
	}
	rest = rest[2:]

	hostEnd, atSign, nonHost := -1, -1, -1
	for i := 0; i < len(rest) && hostEnd == -1; i++ {
		switch rest[i] {
		case '\t', '\n', '\r', ' ', '"', '%', '\'', ';', '<', '>', '\\', '^', '`', '{', '|', '}':
			if nonHost == -1 {
				nonHost = i
			}
		case '#', '/', '?':
			if nonHost == -1 {
				nonHost = i
			}
			hostEnd = i
		case '@':
			atSign = i
			nonHost = -1
		}
	}

	start := 0
	if atSign != -1 {
		start = atSign + 1
	}
	host := rest[start:]
	if nonHost != -1 {
		host = rest[start:nonHost]
	}

	port := ""
	if p := portPattern.FindString(host); p != "" {
		if p != ":" {
			port = p[1:]
		}
		host = host[:len(host)-len(p)]
	}

	hostname := host
	if strings.HasPrefix(hostname, "[") && strings.HasSuffix(hostname, "]") {
		hostname = hostname[1 : len(hostname)-1]
	} else if at := strings.IndexAny(hostname, `/\#?:`); at >= 0 {
		hostname = hostname[:at]
	}
	if len(hostname) > 255 {
		hostname = ""
	}
	hostname = strings.ToLower(hostname)

	if port != "" {
		return hostname + ":" + port
	}
	return hostname
}

// resolveURL implements url.resolve for an absolute base URL.
func resolveURL(base, relative string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return relative
	}
	ref, err := url.Parse(relative)
	if err != nil {
		return relative
	}
	resolved := baseURL.ResolveReference(ref)
	resolved.Host = strings.ToLower(resolved.Host)
	return resolved.String()
}
