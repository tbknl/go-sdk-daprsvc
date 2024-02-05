package daprsvc

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func indexHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "TODO!\n")
}

func (svc *daprSvc) HttpRouter() http.Handler {
	router := httprouter.New()
	router.GET("/", indexHandler)

	return router
}