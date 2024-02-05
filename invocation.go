package daprsvc

import "net/http"

type invocation struct {
	handler http.Handler
}

func detectInvocationRequest(r *http.Request) bool {
	headers := r.Header
	// TODO: Make these header keys configurable!
	mandatoryHeaders := []string{
		"Dapr-Caller-App-Id",
		"Dapr-Callee-App-Id",
	}
	for _, key := range mandatoryHeaders {
		if _, present := headers[key]; !present {
			return false
		}
	}
	return true
}

func (inv *invocation) makeInvocationRequestInterceptor(alternativeHandler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if detectInvocationRequest(r) {
			w.Header().Set("X-Daprsvc-Invocation", "1")
			if inv.handler != nil {
				inv.handler.ServeHTTP(w, r)
			} else {
				http.NotFoundHandler().ServeHTTP(w, r)
			}
		} else {
			alternativeHandler.ServeHTTP(w, r)
		}
	}
}

func (inv *invocation) SetInvocationHandler(handler http.Handler) {
	inv.handler = handler
}
