---
title: "Go: Tracing HTTP requests"
date: 2022-06-04T07:07:15-03:00
draft: false
---

The HTTP protocol is fast, secure and reliable. However, it needs other protocols and services to work properly and **when things don't go well it is necessary to have access to detailed information about the time spent in each step**.

The steps to make an HTTP call are as follows:

1. DNS Lookup
   1. The client sends a DNS query to the DNS server.
   1. The DNS server responds with the IP for the name.
1. TCP connection
   1. The client sends the SYN packet.
   1. Web server responds with SYN-ACK packet.
   1. The client establishes the _(triple handshake)_ connection with the ACK packet.
1. Send
   1. The client sends the HTTP request to the web server.
1. Wait
   1. The client waits until the web server responds to the request.
   1. The web server processes the request and sends the response to the client which receives the HTTP response headers and content.
1. Load
   1. The client loads the response content.
1. Close
   1. The client sends a FIN packet to close the TCP connection.

This is just one of the possible cases for an HTTP request, as we are not addressing persistent connections, connection pooling, or other protocol features.

Go has the **net/http/httptrace** package so that we can collect detailed information about an HTTP request and will be the subject of this article.

After all that said, **how long does it take to translate the name to IP**. Download the [simple-main.go](simple-main.go) file.

```go
func main() {
	var start, dns time.Time
	var took, dnsTook time.Duration

	clientTrace := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) { dns = time.Now() },
		DNSDone:  func(info httptrace.DNSDoneInfo) { dnsTook = time.Since(dns) },
	}
	req, _ := http.NewRequest(http.MethodGet, "https://httpbin.org/", nil)
	clientTraceCtx := httptrace.WithClientTrace(req.Context(), clientTrace)
	req = req.WithContext(clientTraceCtx)
	start = time.Now()
	_, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	took = time.Since(start)
	fmt.Printf("total %dms, dns %dms\n", took.Milliseconds(), dnsTook.Milliseconds())
}
```

Running the code above you will see a result similar to the one below:

```bash
MacBook-Pro:hi thiagocarvalho$ go run main.go
total 612ms, dns 2ms
MacBook-Pro:hi thiagocarvalho$
```

The total request time was 612ms while DNS took 2ms.

## HTTP Client

Before we start talking about _httptrace_ let's remember a little bit about the client of the package [_net/http_](https://pkg.go.dev/net/http) or more specifically about the type [_http.Client_](https://pkg.go.dev/net/http#Client).

```go
type Client struct {
   // Transport specifies the mechanism by which individual
   // HTTP requests are made.
   // If nil, DefaultTransport is used.
   Transport RoundTripper
   ...
   Timeout time.Duration
}
```

For this article we will only contain the _Transport_ field of the _struct_ which is a [_http.RoundTriper_](https://pkg.go.dev/net/http#RoundTripper) which is nothing more than an interface to a function that receives a pointer to [_http.Request_](https://pkg.go.dev/net/http#Request) and returns a pointer to [_http.Response_](https://pkg.go.dev/net/http#Response) it is a mistake. This is quite convenient since basically everything in an HTTP client call involves a request, a response and if there was an error in the process.

## RoundTripper

According to the Go documentation a _RoundTrip_ is `"the ability to execute a single HTTP transaction, obtaining the Response for a given Request."`. In a simplistic way we can say that _RoundTrip_ is nothing more than a _middleware_ of your HTTP call. You generally don't need to worry about this until you have to add a default behavior to **all** calls made by your application, such as serving a _cache_ page of connections (not responses) instead. to fetch the server or implement _retries_.

The _DefaultRoundTrip_ is the following variable:

```go
var DefaultTransport RoundTripper = &Transport{
    Proxy: ProxyFromEnvironment,
    DialContext: defaultTransportDialContext(&net.Dialer{
        Timeout: 30 * time.Second,
        KeepAlive: 30 * time.Second,
    }),
    ForceAttemptHTTP2: true,
    MaxIdleConns: 100,
    IdleConnTimeout: 90 * time.Second,
    TLSHandshakeTimeout: 10 * time.Second,
    ExpectContinueTimeout: 1 * time.Second,
}
```

The transport of an HTTP call is basically what controls the communication between the client and the server, that is, transporting the data in the best possible way. I won't go into details about everything that involves the transport layer of the protocol as it would be too big and I don't have all the necessary knowledge without doing a lot of research, but you can infer a lot just by paying attention to the names as they are quite explicit. **Special thanks to everyone who takes the time to decide the best name for each thing in their code**.

Back to business, _DefaultTransport_ manages network connections, that is, it is responsible for creating new connections as needed, creating a _cache_ to be reused in future requests, as well as honoring the _$HTTP_PROXY_ and _$NO_PROXY_ environment variables.

Last thing about transport for what it's worth quoting something from the documentation about _http.Response.Body_:

```go
...
// The default HTTP client's Transport
// may not reuse HTTP/1.x "keep-alive" TCP connections
// if the Body is not read to completion and closed.
...
Body io.ReadCloser
```

The connection will not be reused until the _body_ is read!

## HTTPTrace

In Go 1.7 the [_net/http/httptrace_](https://pkg.go.dev/net/http/httptrace) package was created to collect information through the lifecycle of an HTTP client call. The package is small and introduces us to a new type [_ClientTrace_](https://pkg.go.dev/net/http/httptrace#ClientTrace) and a function [_WithClientTrace_](https://pkg.go.dev/net/http/httptrace#WithClientTrace).

### ClientTrace Type

```go
type ClientTrace struct {
    ...
    GetConn func(hostPort string)
    ...
    DNSStart func(DNSStartInfo)
    ...
    ConnectDone func(network, addr string, err error)
    ...
    TLSHandshakeStart func()
    ...
    WroteRequest func(WroteRequestInfo)
}
```

The new type introduced by the package is basically a collection of functions that are injected (_hooks_) by various _http.RoundTriper_.

You are responsible for writing the functions, what you get from the package is the automatic injection of the function at the right time with more data. For example, when the library is going to send the packet to resolve the name via DNS automatically the _DNSStart_ field is injected and you get [_DNSStartInfo_](https://pkg.go.dev/net/http/httptrace#DNSStartInfo) and when the response is received from the server automatically the _DNSDone_ field is injected and you get [_DNSDoneInfo_](https://pkg.go.dev/net/http/httptrace#DNSDoneInfo).

### WithClientTrace Function

```go
old := ContextClientTrace(ctx)
trace.compose(old)

ctx = context.WithValue(ctx, clientEventContextKey{}, trace)
if trace.hasNetHooks() {
   ...
   ctx = context.WithValue(ctx, nettrace.TraceKey{}, nt)
}
return ctx
```

The _WithClientTrace_ function does the following:

1. Copy the old context.
1. Create a new context based on the parent context and add the _trace_ values ​​to the context.
1. Checks if there is any _hook_ related to the network layer to be injected into the _ClientTrace_ object.
1. If found, functions are injected into the context.

### Trace and http.Client

Let's now create the code to _trace_ an HTTP call and give us detailed information about the time spent in each step.

1. We need to write the functions to create the _ClientTrace_. Eg.:

   ```go
   func dnsStart(info httptrace.DNSStartInfo) {
      fmt.Printf("quering %q to DNS\n", info.Host)
   }
   func dnsDone(info httptrace.DNSDoneInfo) {
      fmt.Println("DNS info",info)
   }
   ```

1. Instantiate the _ClientTrace_ object with the created functions.

   ```go
   clientTrace := &httptrace.ClientTrace{
      DNSStart: dnsStart,
      DNSDone: dnsDone,
   }
   ```

1. Instantiate the request object.

   ```go
   req, _ := http.NewRequest(http.MethodGet, "https://httpbin.org/redirect-to?url=https://example.com&status_code=307", nil)
   ```

1. Instantiate a new context with _trace_.

   ```go
   clientTraceCtx := httptrace.WithClientTrace(req.Context(), clientTrace)
   ```

1. Associate the new context with the request object.

   ```go
   req = req.WithContext(clientTraceCtx)
   ```

1. Make the HTTP call.

   ```go
   resp, err := http.DefaultClient.Do(req)
   if err != nil {
       log.fatal(err)
   }
   ```

1. Download the response content and close _reader_.

   ```go
   _, err = io.Copy(io.Discard, resp.Body)
   if err != nil {
       log.fatal(err)
   }
   resp.Body.Close()
   ```

Access the complete code at [link](https://go.dev/play/p/ITwAQLcjZSg), read it carefully and run it from your computer. You should see something similar to the content below.

```text
2022/06/03 17:18:15 [TRACE] - starting to create conn to "releases.ubuntu.com:443"
2022/06/03 17:18:15 [TRACE] - quering "releases.ubuntu.com" to DNS
2022/06/03 17:18:15 [TRACE] - ip addresses:
2022/06/03 17:18:15 [TRACE] - - 2620:2d:4000:1::1a
2022/06/03 17:18:15 [TRACE] - - 2001:67c:1562::28
2022/06/03 17:18:15 [TRACE] - - 2620:2d:4000:1::17
2022/06/03 17:18:15 [TRACE] - - 2001:67c:1562::25
2022/06/03 17:18:15 [TRACE] - - 91.189.91.123
2022/06/03 17:18:15 [TRACE] - - 91.189.91.124
2022/06/03 17:18:15 [TRACE] - - 185.125.190.40
2022/06/03 17:18:15 [TRACE] - - 185.125.190.37
2022/06/03 17:18:15 [TRACE] - starting tcp connection to "[2620:2d:4000:1::1a]:443"
2022/06/03 17:18:15 [TRACE] - tcp connection created to [2620:2d:4000:1::1a]:443, err: <nil>
06/03/2022 17:18:15 [TRACE] - starting tls negotiation
2022/06/03 17:18:15 [TRACE] - tls negotiated to "releases.ubuntu.com", error: <nil>
2022/06/03 17:18:15 [TRACE] - connection established. reused: false idle: false idle time: 0ms
2022/06/03 17:18:15 [TRACE] - sending header "Host" and value [releases.ubuntu.com]
2022/06/03 17:18:15 [TRACE] - sending header "User-Agent" and value [Go-http-client/1.1]
2022/06/03 17:18:15 [TRACE] - sending header "Accept-Encoding" and value [gzip]
2022/06/03 17:18:15 [TRACE] - headers written
2022/06/03 17:18:15 [TRACE] - starting to wait for server response
2022/06/03 17:18:16 [TRACE] - got first response byte
2022/06/03 17:18:17 [TRACE] - put conn idle, err: <nil>
Statistics in ms
DNS     Connect TLS     Send  Wait    Transfer Total
269.808 214.251 226.744 0.005 213.591 1605.033 2530.235
```

## Conclusion

HTTP _tracing_ is a very valuable new _feature_ in Go for those who want to have latency information for HTTP calls and write tools for _troubleshooting_ outbound traffic.
