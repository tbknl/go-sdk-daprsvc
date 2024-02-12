package daprsvc

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (svc *daprSvc) HttpHandler() http.Handler {
	router := httprouter.New()

	// Events
	messageHandlerRoutePrefix := "/message"

	router.HandlerFunc(http.MethodGet, "/dapr/subscribe", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		svc.writePubsubConfigData(w, messageHandlerRoutePrefix)
	})

	for _, mwr := range svc.pubsubEntriesWithRoutes() {
		entry := mwr.entry
		router.POST(messageHandlerRoutePrefix+mwr.route, makeEventMessageHandler(entry))
	}

	// Invocation
	routerWithInterceptor := svc.makeInvocationRequestInterceptor(router)

	return routerWithInterceptor
}
