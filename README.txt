# hello

just say hello

## Install

import code

```bash
go get github.com/zuoyebushiwo/hello@latest
```

install cmd

```bash
go install github.com/zuoyebushiwo/hello/cmd/hello@latest
```

## Example

Here's a simple example as follows:

```go
package main

import (
  "fmt"
  "github.com/zuoyebushiwo/hello"
)

func main() {
  result := hello.Hello("jack")
  fmt.Println(result)
}
```