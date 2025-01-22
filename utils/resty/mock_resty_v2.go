package resty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/go-resty/resty/v2"
	"io"
	"net/http"
)

type MockFuncResponse struct {
	Request     *resty.Request
	RawResponse *http.Response
	Body        any
}

type MockFunc struct {
	Method     string
	Path       string
	ResultBody func(header any, requestBody any, param ...QueryParam) (MockFuncResponse, error)
}

type mockRestyClient struct {
	mocks map[string]map[string]MockFunc
}

type mockReadyRestyReq struct {
	mocks  map[string]map[string]MockFunc
	body   any
	header any
}

func (client *mockRestyClient) MakeRequest(ctx context.Context, body any, header any, contentType ...string) ReadyRestyReq {
	return &mockReadyRestyReq{mocks: client.mocks, header: header, body: body}
}

func (m *mockReadyRestyReq) Get(url string, queryParams ...QueryParam) (*resty.Response, error) {
	if mockFunc, ok := m.mocks["GET"][url]; ok {
		header := m.header
		requestBody := m.body

		resultBody, givenError := mockFunc.ResultBody(header, requestBody, queryParams...)
		resultResponse, createErr := CreateMockResponse(resultBody, givenError)
		if createErr != nil {
			return nil, createErr
		}
		if givenError != nil {
			return resultResponse, givenError
		}
		return resultResponse, nil
	}
	return nil, errors.New("mock not found for the requested method and url")
}

func (m *mockReadyRestyReq) Post(url string, queryParams ...QueryParam) (*resty.Response, error) {
	if mockFunc, ok := m.mocks["POST"][url]; ok {
		header := m.header
		requestBody := m.body

		resultBody, givenError := mockFunc.ResultBody(header, requestBody, queryParams...)
		resultResponse, createErr := CreateMockResponse(resultBody, givenError)
		if createErr != nil {
			return nil, createErr
		}
		if givenError != nil {
			return resultResponse, givenError
		}
		return resultResponse, nil
	}
	return nil, errors.New("mock not found for the requested method and url")
}

func (m *mockReadyRestyReq) Put(url string, queryParams ...QueryParam) (*resty.Response, error) {
	if mockFunc, ok := m.mocks["PUT"][url]; ok {
		header := m.header
		requestBody := m.body

		resultBody, givenError := mockFunc.ResultBody(header, requestBody, queryParams...)
		resultResponse, createErr := CreateMockResponse(resultBody, givenError)
		if createErr != nil {
			return nil, createErr
		}
		if givenError != nil {
			return resultResponse, givenError
		}
		return resultResponse, nil
	}
	return nil, errors.New("mock not found for the requested method and url")
}

func (m *mockReadyRestyReq) Delete(url string, queryParams ...QueryParam) (*resty.Response, error) {
	if mockFunc, ok := m.mocks["DELETE"][url]; ok {
		header := m.header
		requestBody := m.body

		resultBody, givenError := mockFunc.ResultBody(header, requestBody, queryParams...)
		resultResponse, createErr := CreateMockResponse(resultBody, givenError)
		if createErr != nil {
			return nil, createErr
		}
		if givenError != nil {
			return resultResponse, givenError
		}
		return resultResponse, nil
	}
	return nil, errors.New("mock not found for the requested method and url")
}

func CreateMockResponse(givenBody MockFuncResponse, givenError error) (*resty.Response, error) {
	var request *resty.Request
	if givenBody.Request == nil {
		request = &resty.Request{}
	} else {
		request = givenBody.Request
	}
	request.Error = givenError

	byteGivenBody, marshalErr := json.Marshal(givenBody.Body)
	if marshalErr != nil {
		return nil, marshalErr
	}

	statusCode := givenBody.RawResponse.StatusCode

	rawResponse := &http.Response{
		Status:     http.StatusText(statusCode),
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(byteGivenBody)),
		Header:     givenBody.RawResponse.Header,
	}
	restyResp := &resty.Response{
		RawResponse: rawResponse,
		Request:     request,
	}
	restyResp.SetBody(byteGivenBody)
	return restyResp, nil
}
