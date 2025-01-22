package resty

import (
	"context"
	"github.com/go-resty/resty/v2"
	"time"
)

type RestyClient interface {
	MakeRequest(ctx context.Context, body any, header any, contentType ...string) ReadyRestyReq
}

type ReadyRestyReq interface {
	Get(url string, queryParams ...QueryParam) (*resty.Response, error)
	Post(url string, queryParams ...QueryParam) (*resty.Response, error)
	Put(url string, queryParams ...QueryParam) (*resty.Response, error)
	Delete(url string, queryParams ...QueryParam) (*resty.Response, error)
}

func NewDefaultRestyClient(trace bool, timeout ...time.Duration) RestyClient {
	restyClient := defaultRestyClient{}
	restyClient.setupClient(trace, 0, timeout...)
	return &restyClient
}

func NewDefaultRestyClientWithTRetryCount(trace bool, retryCount int, timeout ...time.Duration) RestyClient {
	restyClient := defaultRestyClient{}
	restyClient.setupClient(trace, retryCount, timeout...)
	return &restyClient
}

func NewMockRestyClient(mockFuncs []MockFunc) RestyClient {
	mocks := make(map[string]map[string]MockFunc)
	for _, mockFunc := range mockFuncs {
		if _, ok := mocks[mockFunc.Method]; !ok {
			mocks[mockFunc.Method] = make(map[string]MockFunc)
		}
		mocks[mockFunc.Method][mockFunc.Path] = mockFunc
	}
	return &mockRestyClient{
		mocks: mocks,
	}
}

type QueryParam struct {
	Key   string
	Value any
}
