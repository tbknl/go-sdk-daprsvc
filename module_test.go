// Black box test for daprsvc module.
package daprsvc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	daprsvc "github.com/tbknl/go-sdk-daprsvc"
)

// TODO: Put in test-utility package
type EqualJsonResult struct {
	Equal bool
	Err   error
}

var equalJson = EqualJsonResult{true, nil}

func IsEqualJson[T1, T2 []byte | string](json1 T1, json2 T2) EqualJsonResult {
	var r1 interface{}
	var r2 interface{}

	var err error
	err = json.Unmarshal([]byte(json1), &r1)
	if err != nil {
		return EqualJsonResult{
			Equal: false,
			Err:   fmt.Errorf("Error mashalling input 1 :: %s", err.Error()),
		}
	}
	err = json.Unmarshal([]byte(json2), &r2)
	if err != nil {
		return EqualJsonResult{
			Equal: false,
			Err:   fmt.Errorf("Error mashalling input 2 :: %s", err.Error()),
		}
	}

	return EqualJsonResult{
		Equal: reflect.DeepEqual(r1, r2),
		Err:   nil,
	}
}

func doInvocationRequest(handler http.Handler, req *http.Request) *http.Response {
	// TODO: Test with custom dapr headers.
	req.Header.Add("Dapr-Caller-App-Id", "test")
	req.Header.Add("Dapr-Callee-App-Id", "daprsvc")
	wrec := httptest.NewRecorder()
	handler.ServeHTTP(wrec, req)
	return wrec.Result()
}

func Test_InvocationNoHandler(t *testing.T) {
	svc := daprsvc.New()

	result := doInvocationRequest(svc.HttpHandler(), httptest.NewRequest("GET", "/", nil))

	if want, got := "1", result.Header.Get("X-Daprsvc-Invocation"); want != got {
		t.Fatalf("Response to invocation request must always include the invocation header")
	}

	if want, got := http.StatusNotFound, result.StatusCode; want != got {
		t.Fatalf("Invocation request when no invocation handler is set returns status code %d instead of %d", got, want)
	}
}

func Test_Invocation(t *testing.T) {
	svc := daprsvc.New()

	responseText := "Hello"
	mux := http.NewServeMux()
	mux.Handle("/hello", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(responseText))
	}))
	svc.SetInvocationHandler(mux)

	result := doInvocationRequest(svc.HttpHandler(), httptest.NewRequest("GET", "/hello", nil))

	if want, got := "1", result.Header.Get("X-Daprsvc-Invocation"); want != got {
		t.Fatalf("Response to invocation request must always include the invocation header")
	}

	if want, got := http.StatusOK, result.StatusCode; want != got {
		t.Fatalf("Invocation request for registered handler returns status code %d instead of %d", got, want)
	}

	body, _ := io.ReadAll(result.Body)
	if want, got := responseText, string(body); want != got {
		t.Fatalf("Invocation request for registered handler response body is %s instead of %s", got, want)
	}
}

func Test_DaprSubscribeNoMessageHandlers(t *testing.T) {
	svc := daprsvc.New()

	wrec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/dapr/subscribe", nil)
	svc.HttpHandler().ServeHTTP(wrec, req)
	result := wrec.Result()

	if want, got := "application/json", result.Header.Get("Content-Type"); want != got {
		t.Errorf("Expected Content-Type header to be '%s' got '%s'", want, got)
	}

	body, _ := io.ReadAll(result.Body)
	if want, got := "[]", string(body); want != got {
		t.Errorf("Expected body to be '%s' got '%s'", want, got)
	}
}

func Test_DaprSubscribe(t *testing.T) {
	svc := daprsvc.New()
	ps := svc.NewPubsub("servicebus")
	ps.RegisterMessageHandler("order", daprsvc.PubsubOptions{RawPayload: true}, func(ctx context.Context, msg daprsvc.Message) daprsvc.MessageResult {
		return daprsvc.MessageResultSuccess()
	})

	wrec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/dapr/subscribe", nil)
	svc.HttpHandler().ServeHTTP(wrec, req)
	result := wrec.Result()

	if want, got := 200, result.StatusCode; want != got {
		t.Errorf("Expected response status to be '%d' got '%d'", want, got)
	}

	if want, got := "application/json", result.Header.Get("Content-Type"); want != got {
		t.Errorf("Expected Content-Type header to be '%s' got '%s'", want, got)
	}

	body, _ := io.ReadAll(result.Body)
	expected := `[{"pubsubname":"servicebus","topic":"order","route":"/message/servicebus/order","metadata":{"rawPayload":"true"}}]`
	if want, got := equalJson, IsEqualJson(expected, body); want != got {
		t.Errorf("Expected body to equal '%s' got '%s'", expected, string(body))
	}
}

func Test_DaprSubscribeMessagUnknownPubsub(t *testing.T) {
	svc := daprsvc.New()

	req := httptest.NewRequest("POST", "/message/unknown-pubsub/test-topic", bytes.NewBufferString("{}"))
	wrec := httptest.NewRecorder()
	svc.HttpHandler().ServeHTTP(wrec, req)
	result := wrec.Result()

	if want, got := 404, result.StatusCode; want != got {
		t.Errorf("Expected response status to be '%d' got '%d'", want, got)
	}
}

func Test_DaprSubscribeMessagUnknownTopic(t *testing.T) {
	pubsubName := "servicebus"

	svc := daprsvc.New()
	svc.NewPubsub(pubsubName)

	req := httptest.NewRequest("POST", "/message/servicebus/test-topic", bytes.NewBufferString("{}"))
	wrec := httptest.NewRecorder()
	svc.HttpHandler().ServeHTTP(wrec, req)
	result := wrec.Result()

	if want, got := 404, result.StatusCode; want != got {
		t.Errorf("Expected response status to be '%d' got '%d'", want, got)
	}
}

func Test_DaprSubscribeMessageHandlerNoCloudEvent(t *testing.T) {
	pubsubName := "servicebus"
	testTopic := "test-topic"

	svc := daprsvc.New()
	ps := svc.NewPubsub(pubsubName)
	ps.RegisterMessageHandler(testTopic, daprsvc.PubsubOptions{NoCloudEvent: true}, func(ctx context.Context, msg daprsvc.Message) daprsvc.MessageResult {
		data := make(map[string]interface{})
		jsonErr := msg.Json(&data)
		if jsonErr != nil {
			return daprsvc.MessageResultDrop(jsonErr)
		} else if drop, _ := data["DROP"].(bool); drop {
			dropErr := error(nil)
			errMsg, ok := data["DROP_ERROR"].(string)
			if ok {
				dropErr = errors.New(errMsg)
			}
			return daprsvc.MessageResultDrop(dropErr)
		}

		if retry, _ := data["RETRY"].(bool); retry {
			retryErr := error(nil)
			errMsg, ok := data["RETRY_ERROR"].(string)
			if ok {
				retryErr = errors.New(errMsg)
			}
			return daprsvc.MessageResultRetry(retryErr)
		}

		return daprsvc.MessageResultSuccess()
	})

	testCases := []struct {
		body                   map[string]interface{}
		expectedResponseStatus int
		expectedResponseBody   map[string]interface{}
	}{
		{
			body:                   map[string]interface{}{"dummy": 123},
			expectedResponseStatus: 200,
			expectedResponseBody:   map[string]interface{}{"status": "SUCCESS"},
		},
		{
			body:                   map[string]interface{}{"RETRY": true, "RETRY_ERROR": "Something went wrong."},
			expectedResponseStatus: 500,
			expectedResponseBody:   map[string]interface{}{"status": "RETRY", "error": "Something went wrong."},
		},
		{
			body:                   map[string]interface{}{"DROP": true, "DROP_ERROR": "Client error."},
			expectedResponseStatus: 400,
			expectedResponseBody:   map[string]interface{}{"status": "DROP", "error": "Client error."},
		},
	}

	for i, tc := range testCases {
		wrec := httptest.NewRecorder()
		buf, _ := json.Marshal(tc.body)
		req := httptest.NewRequest("POST", "/message/servicebus/test-topic", bytes.NewReader(buf))
		svc.HttpHandler().ServeHTTP(wrec, req)
		result := wrec.Result()

		if want, got := tc.expectedResponseStatus, result.StatusCode; want != got {
			t.Errorf("Test case %d: Expected response status to be '%d' got '%d'", i, want, got)
		}

		if want, got := "application/json", result.Header.Get("Content-Type"); want != got {
			t.Errorf("Test case %d: Expected Content-Type header to be '%s' got '%s'", i, want, got)
		}

		body, _ := io.ReadAll(result.Body)
		expected, _ := json.Marshal(tc.expectedResponseBody)
		if want, got := equalJson, IsEqualJson(expected, body); want != got {
			t.Errorf("Test case %d: Expected body to equal '%s' got '%s'", i, expected, string(body))
		}
	}
}

func Test_DaprSubscribeMessageHandler(t *testing.T) {
	pubsubName := "servicebus"
	testTopic := "test-topic"

	svc := daprsvc.New()
	ps := svc.NewPubsub(pubsubName)
	ps.RegisterMessageHandler(testTopic, daprsvc.PubsubOptions{}, func(ctx context.Context, msg daprsvc.Message) daprsvc.MessageResult {
		data := make(map[string]interface{})
		jsonErr := msg.Json(&data)
		if jsonErr != nil {
			return daprsvc.MessageResultDrop(jsonErr)
		}

		if drop, _ := data["DROP"].(bool); drop {
			dropErr := error(nil)
			errMsg, ok := data["DROP_ERROR"].(string)
			if ok {
				dropErr = errors.New(errMsg)
			}
			return daprsvc.MessageResultDrop(dropErr)
		}

		if retry, _ := data["RETRY"].(bool); retry {
			retryErr := error(nil)
			errMsg, ok := data["RETRY_ERROR"].(string)
			if ok {
				retryErr = errors.New(errMsg)
			}
			return daprsvc.MessageResultRetry(retryErr)
		}

		return daprsvc.MessageResultSuccess()
	})

	testCases := []struct {
		body                   map[string]interface{}
		expectedResponseStatus int
		expectedResponseBody   map[string]interface{}
	}{
		{
			body:                   map[string]interface{}{"dummy": 123},
			expectedResponseStatus: 200,
			expectedResponseBody:   map[string]interface{}{"status": "SUCCESS"},
		},
		{
			body:                   map[string]interface{}{"RETRY": true, "RETRY_ERROR": "Something went wrong."},
			expectedResponseStatus: 500,
			expectedResponseBody:   map[string]interface{}{"status": "RETRY", "error": "Something went wrong."},
		},
		{
			body:                   map[string]interface{}{"DROP": true, "DROP_ERROR": "Client error."},
			expectedResponseStatus: 400,
			expectedResponseBody:   map[string]interface{}{"status": "DROP", "error": "Client error."},
		},
	}

	for i, tc := range testCases {
		wrec := httptest.NewRecorder()
		cloudEvent := map[string]interface{}{
			"id":              "1234-5678",
			"source":          "test-case",
			"specversion":     "1.0",
			"type":            "test-event",
			"datacontenttype": "application/json",
			"data":            tc.body,
			"pubsubname":      pubsubName,
			"topic":           testTopic,
		}
		buf, _ := json.Marshal(cloudEvent)
		req := httptest.NewRequest("POST", "/message/servicebus/test-topic", bytes.NewReader(buf))
		req.Header.Add("Content-type", "application/cloudevents+json")
		svc.HttpHandler().ServeHTTP(wrec, req)
		result := wrec.Result()

		if want, got := tc.expectedResponseStatus, result.StatusCode; want != got {
			t.Errorf("Test case %d: Expected response status to be '%d' got '%d'", i, want, got)
		}

		if want, got := "application/json", result.Header.Get("Content-Type"); want != got {
			t.Errorf("Test case %d: Expected Content-Type header to be '%s' got '%s'", i, want, got)
		}

		body, _ := io.ReadAll(result.Body)
		expected, _ := json.Marshal(tc.expectedResponseBody)
		if want, got := equalJson, IsEqualJson(expected, body); want != got {
			t.Errorf("Test case %d: Expected body to equal '%s' got '%s'", i, expected, string(body))
		}
	}
}
