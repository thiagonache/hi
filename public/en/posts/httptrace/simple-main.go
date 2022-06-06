package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptrace"
	"time"
)

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
