// Black box test for daprsvc module.
package daprsvc_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	daprsvc "github.com/tbknl/go-sdk-daprsvc"
)

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
