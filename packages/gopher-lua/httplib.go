package lua

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"
	"os"
	"io"
	"net/http"
	"compress/gzip"
	"github.com/klauspost/compress/zstd"
)

type empty struct{}

var HttpFuncs = map[string]LGFunction{
	"get":           h_get,
	"delete":        h_delete,
	"head":          h_head,
	"patch":         h_patch,
	"post":          h_post,
	"put":           h_put,
	"request":       h_request,
	"request_batch": h_requestBatch,
}

func OpenHttp(L *LState) int {
	httpmod := L.RegisterModule(HttpLibName, HttpFuncs)
	mt := L.NewTypeMetatable(luaHttpResponseTypeName)
	L.SetField(mt, "__index", L.NewFunction(httpResponseIndex))

	L.SetField(httpmod, "response", mt)
	L.Push(httpmod)
	return 1
}

func h_get(L *LState) int {
	return h_doRequestAndPush(L, "get", L.ToString(1), L.ToTable(2))
}

func h_delete(L *LState) int {
	return h_doRequestAndPush(L, "delete", L.ToString(1), L.ToTable(2))
}

func h_head(L *LState) int {
	return h_doRequestAndPush(L, "head", L.ToString(1), L.ToTable(2))
}

func h_patch(L *LState) int {
	return h_doRequestAndPush(L, "patch", L.ToString(1), L.ToTable(2))
}

func h_post(L *LState) int {
	return h_doRequestAndPush(L, "post", L.ToString(1), L.ToTable(2))
}

func h_put(L *LState) int {
	return h_doRequestAndPush(L, "put", L.ToString(1), L.ToTable(2))
}

func h_request(L *LState) int {
	return h_doRequestAndPush(L, L.ToString(1), L.ToString(2), L.ToTable(3))
}

func h_requestBatch(L *LState) int {
	requests := L.ToTable(1)
	amountRequests := requests.Len()

	errs := make([]error, amountRequests)
	responses := make([]*LUserData, amountRequests)
	sem := make(chan empty, amountRequests)

	i := 0

	requests.ForEach(func(_ LValue, value LValue) {
		requestTable := toTable(value)

		if requestTable != nil {
			method := requestTable.RawGet(LNumber(1)).String()
			url := requestTable.RawGet(LNumber(2)).String()
			options := toTable(requestTable.RawGet(LNumber(3)))

			go func(i int, L *LState, method string, url string, options *LTable) {
				response, err := h_doRequest(L, method, url, options)

				if err == nil {
					errs[i] = nil
					responses[i] = response
				} else {
					errs[i] = err
					responses[i] = nil
				}

				sem <- empty{}
			}(i, L, method, url, options)
		} else {
			errs[i] = errors.New("Request must be a table")
			responses[i] = nil
			sem <- empty{}
		}

		i = i + 1
	})

	for i = 0; i < amountRequests; i++ {
		<-sem
	}

	hasErrors := false
	errorsTable := L.NewTable()
	responsesTable := L.NewTable()
	for i = 0; i < amountRequests; i++ {
		if errs[i] == nil {
			responsesTable.Append(responses[i])
			errorsTable.Append(LNil)
		} else {
			responsesTable.Append(LNil)
			errorsTable.Append(LString(fmt.Sprintf("%s", errs[i])))
			hasErrors = true
		}
	}

	if hasErrors {
		L.Push(responsesTable)
		L.Push(errorsTable)
		return 2
	} else {
		L.Push(responsesTable)
		return 1
	}
}

func h_doRequest(L *LState, method string, url string, options *LTable) (*LUserData, error) {
	outputFile := ""
	req, err := http.NewRequest(strings.ToUpper(method), url, nil)
	if err != nil {
		return nil, err
	}
	// req.Header.Set("Accept-Encoding", "zstd")
	// req.Header.Set("Accept-Encoding", "gzip")
	// req.Header.Set("Accept-Encoding", "gzip, zstd")

	if ctx := L.Context(); ctx != nil {
		req = req.WithContext(ctx)
	}

	if options != nil {
		if reqCookies, ok := options.RawGet(LString("cookies")).(*LTable); ok {
			reqCookies.ForEach(func(key LValue, value LValue) {
				req.AddCookie(&http.Cookie{Name: key.String(), Value: value.String()})
			})
		}

		switch reqQuery := options.RawGet(LString("query")).(type) {
		case LString:
			req.URL.RawQuery = reqQuery.String()
		}

		body := options.RawGet(LString("body"))
		if _, ok := body.(LString); !ok {
			// "form" is deprecated.
			body = options.RawGet(LString("form"))
			// Only set the Content-Type to application/x-www-form-urlencoded
			// when someone uses "form", not for "body".
			if _, ok := body.(LString); ok {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
		}

		switch reqBody := body.(type) {
		case LString:
			body := reqBody.String()
			req.ContentLength = int64(len(body))
			req.Body = ioutil.NopCloser(strings.NewReader(body))
		}

		reqTimeout := options.RawGet(LString("timeout"))
		if reqTimeout != LNil {
			duration := time.Duration(0)
			switch reqTimeout.(type) {
			case LNumber:
				duration = time.Second * time.Duration(int(reqTimeout.(LNumber)))
			case LString:
				duration, err = time.ParseDuration(string(reqTimeout.(LString)))
				if err != nil {
					return nil, err
				}
			}
			ctx, cancel := context.WithTimeout(req.Context(), duration)
			req = req.WithContext(ctx)
			defer cancel()
		}

		// Output Body to filename
		reqFileName := options.RawGet(LString("output"))
		if reqFileName != LNil {
			switch reqFileName.(type) {
			case LString:
				outputFile = reqFileName.String()
			}
		}

		// Basic auth
		if reqAuth, ok := options.RawGet(LString("auth")).(*LTable); ok {
			user := reqAuth.RawGetString("user")
			pass := reqAuth.RawGetString("pass")
			if !LVIsFalse(user) && !LVIsFalse(pass) {
				req.SetBasicAuth(user.String(), pass.String())
			} else {
				return nil, fmt.Errorf("auth table must contain no nil user and pass fields")
			}
		}

		// Set these last. That way the code above doesn't overwrite them.
		if reqHeaders, ok := options.RawGet(LString("headers")).(*LTable); ok {
			reqHeaders.ForEach(func(key LValue, value LValue) {
				req.Header.Set(key.String(), value.String())
			})
		}
	}

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	disposition := res.Header.Get("Content-Disposition")
	if outputFile == "" {
		if strings.Contains(disposition, "filename") {
			filename := strings.Split(disposition, "filename=")[1]
			filename = strings.Trim(filename, "\"")
			outputFile = filename
		}
	}

	if outputFile == "" {
		var body []byte
		if res.Header.Get("Content-Encoding") == "gzip" {
			gzipReader, err := gzip.NewReader(res.Body)
			if err != nil {
				return nil, err
			}
			defer gzipReader.Close()
			body, err = ioutil.ReadAll(gzipReader)
		} else if res.Header.Get("Content-Encoding") == "zstd" {
			zstdReader, err := zstd.NewReader(res.Body)
			if err != nil {
				return nil, err
			}
			defer zstdReader.Close()
			body, err = ioutil.ReadAll(zstdReader)
		} else {
			body, err = ioutil.ReadAll(res.Body)
		}
		if err != nil {
			return nil, err
		}
		return newHttpResponse(res, &body, len(body), L), nil
	} else {
		var body []byte = nil
		out, err := os.Create(outputFile)
		if err != nil {
			return nil, err
		}
		defer out.Close()
		_, err = io.Copy(out, res.Body)
		if err != nil {
			return nil, err
		}
		return newHttpResponse(res, &body, 0, L), nil
	}

	
}

func h_doRequestAndPush(L *LState, method string, url string, options *LTable) int {
	response, err := h_doRequest(L, method, url, options)

	if err != nil {
		L.Push(LNil)
		L.Push(LString(fmt.Sprintf("%s", err)))
		return 2
	}

	L.Push(response)
	return 1
}

func toTable(v LValue) *LTable {
	if lv, ok := v.(*LTable); ok {
		return lv
	}
	return nil
}

const luaHttpResponseTypeName = "http.response"

type luaHttpResponse struct {
	res      *http.Response
	body     LString
	bodySize int
}

func newHttpResponse(res *http.Response, body *[]byte, bodySize int, L *LState) *LUserData {
	ud := L.NewUserData()
	ud.Value = &luaHttpResponse{
		res:      res,
		body:     LString(*body),
		bodySize: bodySize,
	}
	L.SetMetatable(ud, L.GetTypeMetatable(luaHttpResponseTypeName))
	return ud
}

func checkHttpResponse(L *LState) *luaHttpResponse {
	ud := L.CheckUserData(1)
	if v, ok := ud.Value.(*luaHttpResponse); ok {
		return v
	}
	L.ArgError(1, "http.response expected")
	return nil
}

func httpResponseIndex(L *LState) int {
	res := checkHttpResponse(L)

	switch L.CheckString(2) {
	case "headers":
		return httpResponseHeaders(res, L)
	case "cookies":
		return httpResponseCookies(res, L)
	case "status_code":
		return httpResponseStatusCode(res, L)
	case "url":
		return httpResponseUrl(res, L)
	case "body":
		return httpResponseBody(res, L)
	case "body_size":
		return httpResponseBodySize(res, L)
	}

	return 0
}

func httpResponseHeaders(res *luaHttpResponse, L *LState) int {
	headers := L.NewTable()
	for key, _ := range res.res.Header {
		headers.RawSetString(key, LString(res.res.Header.Get(key)))
	}
	L.Push(headers)
	return 1
}

func httpResponseCookies(res *luaHttpResponse, L *LState) int {
	cookies := L.NewTable()
	for _, cookie := range res.res.Cookies() {
		cookies.RawSetString(cookie.Name, LString(cookie.Value))
	}
	L.Push(cookies)
	return 1
}

func httpResponseStatusCode(res *luaHttpResponse, L *LState) int {
	L.Push(LNumber(res.res.StatusCode))
	return 1
}

func httpResponseUrl(res *luaHttpResponse, L *LState) int {
	L.Push(LString(res.res.Request.URL.String()))
	return 1
}

func httpResponseBody(res *luaHttpResponse, L *LState) int {
	L.Push(res.body)
	return 1
}

func httpResponseBodySize(res *luaHttpResponse, L *LState) int {
	L.Push(LNumber(res.bodySize))
	return 1
}
