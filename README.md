go-sdk-daprsvc
==============

Alternative Dapr service SDK for Go.


## Rationale

Considerations for creating an alternative implementation for the Dapr service SDK:
* Allow development with any (`net/http` compatible) http router/mux.
* Leave control of the http server with the application developer.
    * Allow the application to deal with graceful shutdown (including choice of time-out duration).
    * Allow application to use either `ListenAndServe` or separate `Listen` and `Serve`.
* Reduce module dependencies to a minimum.

## Basic usage example

```shell
go get github.com/tbknl/go-sdk-daprsvc
```

```go
package main

import (
    daprsvc "github.com/tbknl/go-sdk-daprsvc"
)

func main() {
	appPort := os.Getenv("APP_PORT")

    svc := daprsvc.New()
    handler := svc.HttpHandler()

    log.Fatal(http.ListenAndServe(":" + appPort, svc.HttpHandler()))
}
```

## Setting up a Dapr service

To set up a Dapr service:
* Create a new service instance: `svc := daprsvc.New()`
* Register handlers for the different types of services that your application provides. E.g.: `svc.SetInvocationHandler(myRouter)`
* Create an http handler for the dapr service: `handler := svc.HttpHandler()`
* Create an http server (using Go's built-in `net/http` module) to start listening on the port where Dapr knows to reach your service (typically the `APP_PORT` environment variable is set accordingly), using the service handler to serve incoming requests. E.g.: `http.ListenAndServe(":" + port, handler)`.

See the [basic usage example](#basic-usage-example)

## Features

### Invocation

An http handler compatible with Go's `http.Handler` can be registered to handle incoming http [service invocation requests](https://docs.dapr.io/developing-applications/building-blocks/service-invocation/service-invocation-overview/). The handler may be a router from any external package that can provide an `net/http` compatible handler.

Example using the `httprouter` package for handling requests:
```go
router := httprouter.New()
router.GET("/hello", func (w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
        io.WriteString(w, "Hello world!")
})

svc.SetInvocationHandler(router)
```

### Pub-sub

Register message handlers for subscriptions to topics on a Dapr pubsub component. The endpoint `/dapr/subscribe` will automatically expose all subscription information to the Dapr daemon.

Example:
```go
myPubsub := svc.NewPubsub("my-pubsub")

myPubsub.RegisterMessageHandler("orders", daprsvc.PubsubOptions{}, func(ctx context.Context, msg daprsvc.Message) daprsvc.MessageResult {
    data := make(map[string]interface{})
    jsonErr := msg.Json(&data)
    if jsonErr != nil {
        return daprsvc.MessageResultDrop(jsonErr)
    }
    fmt.Printf("Order %s received!\n", order["id"])

    return daprsvc.MessageResultSuccess()
})
```

