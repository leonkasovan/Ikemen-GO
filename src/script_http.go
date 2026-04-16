package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// httpState holds shared state for the http Lua module.
type httpState struct {
	mu          sync.Mutex
	client      *http.Client
	cookieFile  string
	lastURL     string
	defaultUA   string
}

var globalHTTP = &httpState{
	defaultUA: "Mozilla/5.0 Ikemen-GO/1.0",
}

func init() {
	jar, _ := cookiejar.New(nil)
	globalHTTP.client = &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}
}

// httpOptions parsed from the Lua options table.
type httpOptions struct {
	userAgent       string
	followRedirects bool
	timeout         int // seconds, 0 = use default
	headers         map[string]string
}

func parseHTTPOptions(l *lua.LState, argi int) httpOptions {
	opts := httpOptions{
		userAgent:       globalHTTP.defaultUA,
		followRedirects: true,
	}
	if l.GetTop() < argi {
		return opts
	}
	tbl := l.ToTable(argi)
	if tbl == nil {
		return opts
	}

	if v := tbl.RawGetString("user_agent"); v != lua.LNil {
		opts.userAgent = v.String()
	}
	if v := tbl.RawGetString("follow_redirects"); v != lua.LNil {
		if b, ok := v.(lua.LBool); ok {
			opts.followRedirects = bool(b)
		}
	}
	if v := tbl.RawGetString("timeout"); v != lua.LNil {
		if n, ok := v.(lua.LNumber); ok {
			opts.timeout = int(n)
		}
	}
	if v := tbl.RawGetString("headers"); v != lua.LNil {
		if ht, ok := v.(*lua.LTable); ok {
			opts.headers = make(map[string]string)
			ht.ForEach(func(k, val lua.LValue) {
				opts.headers[k.String()] = val.String()
			})
		}
	}
	return opts
}

// buildClient returns an *http.Client configured per the given options.
// It reuses the global client's cookie jar.
func buildClient(opts httpOptions) *http.Client {
	globalHTTP.mu.Lock()
	jar := globalHTTP.client.Jar
	globalHTTP.mu.Unlock()

	timeout := 30 * time.Second
	if opts.timeout > 0 {
		timeout = time.Duration(opts.timeout) * time.Second
	}

	c := &http.Client{
		Jar:     jar,
		Timeout: timeout,
	}
	if !opts.followRedirects {
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return c
}

// doRequest performs the HTTP request and pushes (body, status, headers) onto the Lua stack.
func doRequest(l *lua.LState, req *http.Request, opts httpOptions) int {
	req.Header.Set("User-Agent", opts.userAgent)
	for k, v := range opts.headers {
		req.Header.Set(k, v)
	}

	client := buildClient(opts)
	resp, err := client.Do(req)
	if err != nil {
		l.Push(lua.LString(""))
		l.Push(lua.LNumber(0))
		l.Push(l.NewTable())
		// Log the error for debugging
		LogMessage("http request error: %v", err)
		return 3
	}
	defer resp.Body.Close()

	// Track last effective URL.
	globalHTTP.mu.Lock()
	globalHTTP.lastURL = resp.Request.URL.String()
	globalHTTP.mu.Unlock()

	// Limit body read to 64 MB to prevent OOM.
	const maxBody = 64 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		l.Push(lua.LString(""))
		l.Push(lua.LNumber(float64(resp.StatusCode)))
		l.Push(l.NewTable())
		return 3
	}

	// Build headers table.
	hdrs := l.NewTable()
	for k, vals := range resp.Header {
		if len(vals) == 1 {
			hdrs.RawSetString(k, lua.LString(vals[0]))
		} else {
			hdrs.RawSetString(k, lua.LString(strings.Join(vals, ", ")))
		}
	}

	l.Push(lua.LString(string(body)))
	l.Push(lua.LNumber(float64(resp.StatusCode)))
	l.Push(hdrs)
	return 3
}

// registerHTTPLib registers the "http" module table on the Lua state.
func registerHTTPLib(l *lua.LState) {
	mod := l.NewTable()

	// http.get(url [, options]) -> body, status, headers
	l.SetField(mod, "get", l.NewFunction(func(l *lua.LState) int {
		rawURL := l.ToString(1)
		if rawURL == "" {
			l.ArgError(1, "url string expected")
			return 0
		}
		opts := parseHTTPOptions(l, 2)

		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			l.Push(lua.LString(""))
			l.Push(lua.LNumber(0))
			l.Push(l.NewTable())
			LogMessage("http.get: invalid request: %v", err)
			return 3
		}
		return doRequest(l, req, opts)
	}))

	// http.post(url, body [, options]) -> body, status, headers
	l.SetField(mod, "post", l.NewFunction(func(l *lua.LState) int {
		rawURL := l.ToString(1)
		if rawURL == "" {
			l.ArgError(1, "url string expected")
			return 0
		}
		reqBody := l.ToString(2)
		opts := parseHTTPOptions(l, 3)

		req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(reqBody))
		if err != nil {
			l.Push(lua.LString(""))
			l.Push(lua.LNumber(0))
			l.Push(l.NewTable())
			LogMessage("http.post: invalid request: %v", err)
			return 3
		}
		// Default Content-Type for POST if not set by caller.
		if _, ok := opts.headers["Content-Type"]; !ok {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return doRequest(l, req, opts)
	}))

	// http.head(url [, options]) -> body, status, headers
	l.SetField(mod, "head", l.NewFunction(func(l *lua.LState) int {
		rawURL := l.ToString(1)
		if rawURL == "" {
			l.ArgError(1, "url string expected")
			return 0
		}
		opts := parseHTTPOptions(l, 2)

		req, err := http.NewRequest(http.MethodHead, rawURL, nil)
		if err != nil {
			l.Push(lua.LString(""))
			l.Push(lua.LNumber(0))
			l.Push(l.NewTable())
			LogMessage("http.head: invalid request: %v", err)
			return 3
		}
		return doRequest(l, req, opts)
	}))

	// http.set_cookie(filepath)
	l.SetField(mod, "set_cookie", l.NewFunction(func(l *lua.LState) int {
		fp := l.ToString(1)
		if fp == "" {
			l.ArgError(1, "filepath string expected")
			return 0
		}
		globalHTTP.mu.Lock()
		globalHTTP.cookieFile = fp
		// Reset the jar so new cookies start fresh from this file.
		jar, _ := cookiejar.New(nil)
		globalHTTP.client.Jar = jar
		globalHTTP.mu.Unlock()
		return 0
	}))

	// http.clear_cookies()
	l.SetField(mod, "clear_cookies", l.NewFunction(func(l *lua.LState) int {
		globalHTTP.mu.Lock()
		globalHTTP.cookieFile = ""
		globalHTTP.lastURL = ""
		jar, _ := cookiejar.New(nil)
		globalHTTP.client.Jar = jar
		globalHTTP.mu.Unlock()
		return 0
	}))

	// http.get_last_url() -> string
	l.SetField(mod, "get_last_url", l.NewFunction(func(l *lua.LState) int {
		globalHTTP.mu.Lock()
		u := globalHTTP.lastURL
		globalHTTP.mu.Unlock()
		l.Push(lua.LString(u))
		return 1
	}))

	// http.url_encode(str) -> string
	l.SetField(mod, "url_encode", l.NewFunction(func(l *lua.LState) int {
		s := l.ToString(1)
		l.Push(lua.LString(url.QueryEscape(s)))
		return 1
	}))

	// http.url_decode(str) -> string
	l.SetField(mod, "url_decode", l.NewFunction(func(l *lua.LState) int {
		s := l.ToString(1)
		decoded, err := url.QueryUnescape(s)
		if err != nil {
			l.Push(lua.LString(s))
		} else {
			l.Push(lua.LString(decoded))
		}
		return 1
	}))

	// http.parse_url(url) -> table {scheme, host, port, path, query}
	l.SetField(mod, "parse_url", l.NewFunction(func(l *lua.LState) int {
		rawURL := l.ToString(1)
		u, err := url.Parse(rawURL)
		if err != nil {
			l.Push(lua.LNil)
			return 1
		}
		t := l.NewTable()
		t.RawSetString("scheme", lua.LString(u.Scheme))
		t.RawSetString("host", lua.LString(u.Hostname()))
		port := u.Port()
		if port != "" {
			t.RawSetString("port", lua.LString(port))
		} else {
			t.RawSetString("port", lua.LString(""))
		}
		t.RawSetString("path", lua.LString(u.Path))
		t.RawSetString("query", lua.LString(u.RawQuery))
		l.Push(t)
		return 1
	}))

	l.SetGlobal("http", mod)

	fmt.Println("http library registered")
}
