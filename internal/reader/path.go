package reader

import (
	"strings"
	"syscall"
)

// The original resolves paths with the POSIX flavour of node's path module;
// these helpers reproduce its results exactly, including its handling of
// empty segments, "." and "..".

// cwd returns the working directory as the operating system reports it,
// without consulting $PWD.
func cwd() string {
	dir, err := syscall.Getwd()
	if err != nil {
		return "/"
	}
	return dir
}

func isAbsolute(p string) bool {
	return strings.HasPrefix(p, "/")
}

// normalizeString resolves "." and ".." segments of p and drops empty ones.
func normalizeString(p string, allowAboveRoot bool) string {
	res := ""
	lastSegmentLength := 0
	lastSlash := -1
	dots := 0
	var code byte
	for i := 0; i <= len(p); i++ {
		if i < len(p) {
			code = p[i]
		} else if code == '/' {
			break
		} else {
			code = '/'
		}

		switch {
		case code == '/':
			switch {
			case lastSlash == i-1 || dots == 1:
				// empty segment or "."
			case dots == 2:
				if len(res) < 2 || lastSegmentLength != 2 || !strings.HasSuffix(res, "..") {
					if len(res) > 2 {
						if at := strings.LastIndex(res, "/"); at == -1 {
							res = ""
							lastSegmentLength = 0
						} else {
							res = res[:at]
							lastSegmentLength = len(res) - 1 - strings.LastIndex(res, "/")
						}
						lastSlash = i
						dots = 0
						continue
					} else if len(res) != 0 {
						res = ""
						lastSegmentLength = 0
						lastSlash = i
						dots = 0
						continue
					}
				}
				if allowAboveRoot {
					if len(res) > 0 {
						res += "/.."
					} else {
						res = ".."
					}
					lastSegmentLength = 2
				}
			default:
				if len(res) > 0 {
					res += "/" + p[lastSlash+1:i]
				} else {
					res = p[lastSlash+1 : i]
				}
				lastSegmentLength = i - lastSlash - 1
			}
			lastSlash = i
			dots = 0
		case code == '.' && dots != -1:
			dots++
		default:
			dots = -1
		}
	}
	return res
}

// resolvePath implements path.resolve.
func resolvePath(paths ...string) string {
	resolved := ""
	resolvedAbsolute := false
	for i := len(paths) - 1; i >= -1 && !resolvedAbsolute; i-- {
		p := ""
		if i >= 0 {
			p = paths[i]
		} else {
			p = cwd()
		}
		if p == "" {
			continue
		}
		resolved = p + "/" + resolved
		resolvedAbsolute = p[0] == '/'
	}
	resolved = normalizeString(resolved, !resolvedAbsolute)
	if resolvedAbsolute {
		return "/" + resolved
	}
	if resolved == "" {
		return "."
	}
	return resolved
}

// normalizePosix implements path.normalize.
func normalizePosix(p string) string {
	if p == "" {
		return "."
	}
	absolute := p[0] == '/'
	trailingSeparator := p[len(p)-1] == '/'
	p = normalizeString(p, !absolute)
	if p == "" {
		if absolute {
			return "/"
		}
		if trailingSeparator {
			return "./"
		}
		return "."
	}
	if trailingSeparator {
		p += "/"
	}
	if absolute {
		return "/" + p
	}
	return p
}

// joinPath implements path.join.
func joinPath(paths ...string) string {
	var parts []string
	for _, p := range paths {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	return normalizePosix(strings.Join(parts, "/"))
}

// dirname implements path.dirname.
func dirname(p string) string {
	if p == "" {
		return "."
	}
	hasRoot := p[0] == '/'
	end := -1
	matchedSlash := true
	for i := len(p) - 1; i >= 1; i-- {
		if p[i] == '/' {
			if !matchedSlash {
				end = i
				break
			}
		} else {
			matchedSlash = false
		}
	}
	if end == -1 {
		if hasRoot {
			return "/"
		}
		return "."
	}
	if hasRoot && end == 1 {
		return "//"
	}
	return p[:end]
}

// relativePath implements path.relative.
func relativePath(from, to string) string {
	if from == to {
		return ""
	}
	from = resolvePath(from)
	to = resolvePath(to)
	if from == to {
		return ""
	}

	const fromStart, toStart = 1, 1
	fromEnd := len(from)
	fromLen := fromEnd - fromStart
	toLen := len(to) - toStart
	length := min(fromLen, toLen)
	lastCommonSep := -1
	i := 0
	for ; i < length; i++ {
		fromCode := from[fromStart+i]
		if fromCode != to[toStart+i] {
			break
		} else if fromCode == '/' {
			lastCommonSep = i
		}
	}
	if i == length {
		if toLen > length {
			if to[toStart+i] == '/' {
				return to[toStart+i+1:]
			}
			if i == 0 {
				return to[toStart+i:]
			}
		} else if fromLen > length {
			if from[fromStart+i] == '/' {
				lastCommonSep = i
			} else if i == 0 {
				lastCommonSep = 0
			}
		}
	}

	out := ""
	for i = fromStart + lastCommonSep + 1; i <= fromEnd; i++ {
		if i == fromEnd || from[i] == '/' {
			if out == "" {
				out = ".."
			} else {
				out += "/.."
			}
		}
	}
	return out + to[toStart+lastCommonSep:]
}
