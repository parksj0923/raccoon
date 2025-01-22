package resty

import (
	"context"
	"fmt"
	"github.com/go-resty/resty/v2"
	"net"
	"net/http"
	urlTool "net/url"
	"strings"
	"time"
)

type defaultRestyClient struct {
	restyClient *resty.Client
}

func (client *defaultRestyClient) MakeRequest(ctx context.Context, body any, header any, contentType ...string) ReadyRestyReq {
	request := client.restyClient.R().SetContext(ctx)
	if body != nil {
		request.SetBody(body)
	}

	if len(contentType) > 0 {
		request.SetHeader("Content-Type", contentType[0])
		request.SetHeader("Accept", contentType[0])
	} else {
		request.SetHeader("Content-Type", "application/json")
		request.SetHeader("Accept", "application/json")
	}

	if header != nil {
		request.SetHeaders(header.(map[string]string))
	}
	return &defaultReadyRestyReq{request: request}
}

func (client *defaultRestyClient) setupClient(trace bool, retry int, timeout ...time.Duration) {
	restyClient := client.restyClient
	restyClient = resty.New()
	restyClient.SetRetryCount(retry)
	restyClient.SetTimeout(10 * time.Second)
	if len(timeout) > 0 {
		restyClient.SetTimeout(timeout[0])
	}
	restyClient.SetRetryWaitTime(time.Second)
	restyClient.SetRetryMaxWaitTime(5 * time.Second)
	restyClient.AddRetryCondition(func(response *resty.Response, err error) bool {
		return response.StatusCode() >= 500 || err != nil
	})

	defaultTransport := &http.Transport{}
	defaultTransport.DialContext = (&net.Dialer{}).DialContext

	defaultTransport.MaxIdleConns = 100
	defaultTransport.MaxIdleConnsPerHost = 100

	restyClient.SetTransport(defaultTransport)

	if trace {
		restyClient.EnableTrace()
	}

	client.restyClient = restyClient
}

type defaultReadyRestyReq struct {
	request *resty.Request
}

func (req *defaultReadyRestyReq) makeUrl(url string, queryParams ...QueryParam) string {
	if len(queryParams) == 0 {
		return url
	}
	var queryString []string
	for _, query := range queryParams {
		strValue := fmt.Sprintf("%v", query.Value)
		queryString = append(queryString, fmt.Sprintf("%s=%s", urlTool.QueryEscape(query.Key), urlTool.QueryEscape(strValue)))
	}
	return fmt.Sprintf("%s?%s", url, strings.Join(queryString, "&"))
}

func (req *defaultReadyRestyReq) Get(url string, queryParams ...QueryParam) (*resty.Response, error) {
	convertedURL := req.makeUrl(url, queryParams...)
	return req.request.Get(convertedURL)
}
func (req *defaultReadyRestyReq) Post(url string, queryParams ...QueryParam) (*resty.Response, error) {
	convertedURL := req.makeUrl(url, queryParams...)
	return req.request.Post(convertedURL)
}
func (req *defaultReadyRestyReq) Put(url string, queryParams ...QueryParam) (*resty.Response, error) {
	convertedURL := req.makeUrl(url, queryParams...)
	return req.request.Put(convertedURL)
}
func (req *defaultReadyRestyReq) Delete(url string, queryParams ...QueryParam) (*resty.Response, error) {
	convertedURL := req.makeUrl(url, queryParams...)
	return req.request.Delete(convertedURL)
}
