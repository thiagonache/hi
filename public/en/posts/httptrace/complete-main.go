package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptrace"
	"time"
)

type stats struct {
	client          http.Client
	connStartAt     time.Time
	connTook        time.Duration
	dnsStartAt      time.Time
	dnsTook         time.Duration
	sendStartAt     time.Time
	sendTook        time.Duration
	tlsStartAt      time.Time
	tlsTook         time.Duration
	totalStartAt    time.Time
	totalTook       time.Duration
	transferStartAt time.Time
	transferTook    time.Duration
	waitStartAt     time.Time
	waitTook        time.Duration
}

func newStats() *stats {
	return &stats{
		client: http.Client{},
	}
}

func (s *stats) getConn(hostPort string) {
	s.totalStartAt = time.Now()
	log.Printf("[TRACE] - starting to create conn to %q\n", hostPort)
}

func (s *stats) dnsStart(info httptrace.DNSStartInfo) {
	s.dnsStartAt = time.Now()
	log.Printf("[TRACE] - quering %q to DNS\n", info.Host)
}

func (s *stats) dnsDone(info httptrace.DNSDoneInfo) {
	s.dnsTook = time.Since(s.dnsStartAt)
	if info.Err != nil {
		return
	}
	log.Println("[TRACE] - ip addresses:")
	for _, addr := range info.Addrs {
		log.Printf("[TRACE] - - %s\n", &addr.IP)
	}
}

func (s *stats) connectStart(network, addr string) {
	s.connStartAt = time.Now()
	log.Printf("[TRACE] - starting %s connection to %q\n", network, addr)
}

func (s *stats) connectDone(network, addr string, err error) {
	s.connTook = time.Since(s.connStartAt)
	if err != nil {
		return
	}
	log.Printf("[TRACE] - %s connection created to %s, err: %+v\n", network, addr, err)
}

func (s *stats) tlsStart() {
	s.tlsStartAt = time.Now()
	log.Println("[TRACE] - starting tls negotiation")
}

func (s *stats) tlsDone(cs tls.ConnectionState, err error) {
	s.tlsTook = time.Since(s.tlsStartAt)
	if err != nil {
		return
	}
	log.Printf("[TRACE] - tls negotiated to %q, error: %+v\n", cs.ServerName, err)
}

func (s stats) gotConn(info httptrace.GotConnInfo) {
	log.Printf("[TRACE] - connection established. reused: %t idle: %t idle time: %dms\n", info.Reused, info.WasIdle, info.IdleTime.Milliseconds())
}

func (s *stats) wroteHeaderField(key string, value []string) {
	s.sendStartAt = time.Now()
	log.Printf("[TRACE] - sending header %q and value %s\n", key, value)
}

func (s *stats) wroteHeaders() {
	s.sendTook = time.Since(s.sendStartAt)
	log.Println("[TRACE] - headers written")
}

func (s *stats) wroteRequest(info httptrace.WroteRequestInfo) {
	s.waitStartAt = time.Now()
	if info.Err != nil {
		return
	}
	log.Println("[TRACE] - starting to wait for server response")
}

func (s *stats) gotFirstResponseByte() {
	s.waitTook = time.Since(s.waitStartAt)
	s.transferStartAt = time.Now()
	log.Println("[TRACE] - got first response byte")
}

func (s *stats) putIdleConn(err error) {
	s.totalTook = time.Since(s.totalStartAt)
	s.transferTook = time.Since(s.transferStartAt)
	if err != nil {
		return
	}
	log.Printf("[TRACE] - put conn idle, err: %+v", err)
}

func main() {
	s := newStats()
	clientTrace := &httptrace.ClientTrace{
		GetConn:              s.getConn,
		DNSStart:             s.dnsStart,
		DNSDone:              s.dnsDone,
		ConnectStart:         s.connectStart,
		ConnectDone:          s.connectDone,
		TLSHandshakeStart:    s.tlsStart,
		TLSHandshakeDone:     s.tlsDone,
		GotConn:              s.gotConn,
		WroteHeaderField:     s.wroteHeaderField,
		WroteHeaders:         s.wroteHeaders,
		WroteRequest:         s.wroteRequest,
		GotFirstResponseByte: s.gotFirstResponseByte,
		PutIdleConn:          s.putIdleConn,
	}
	// 1.2G file
	//req, _ := http.NewRequest(http.MethodGet, "https://releases.ubuntu.com/20.04/ubuntu-20.04.4-live-server-amd64.iso", nil)
	// 2.5M file
	req, _ := http.NewRequest(http.MethodGet, "https://releases.ubuntu.com/20.04/ubuntu-20.04.4-live-server-amd64.iso.zsync", nil)
	clientTraceCtx := httptrace.WithClientTrace(req.Context(), clientTrace)
	req = req.WithContext(clientTraceCtx)
	resp, err := s.client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	resp.Body.Close()
	fmt.Println("Statistics in ms")
	fmt.Println("DNS\tConnect\tTLS\tSend\tWait\tTransfer\tTotal")
	fmt.Printf("%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\n",
		float64(s.dnsTook.Nanoseconds())/1000000.0,
		float64(s.connTook.Nanoseconds())/1000000.0,
		float64(s.tlsTook.Nanoseconds())/1000000.0,
		float64(s.sendTook.Nanoseconds())/1000000.0,
		float64(s.waitTook.Nanoseconds())/1000000.0,
		float64(s.transferTook.Nanoseconds())/1000000.0,
		float64(s.totalTook.Nanoseconds())/1000000.0,
	)
}
