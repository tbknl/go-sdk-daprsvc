go-sdk-daprsvc
==============

Alternative Dapr service SDK for Go.

## Basic usage example

```go
package main

import (
    "github.com/tbknl/go-sdk-daprsvc"
)

func main() {
    daprSvc := daprsvc.NewHttpRouter()

    log.Fatal(http.ListenAndServe(":8080", daprSvc))
}
```
