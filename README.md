go-sdk-daprsvc
==============

Alternative Dapr service SDK for Go.

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
    daprSvc := daprsvc.New()
    router := daprSvc.HttpRouter()

    log.Fatal(http.ListenAndServe(":8080", router))
}
```
