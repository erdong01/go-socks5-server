package httpproxy

import (
	"bufio"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// 超时设置
const (
	dialTimeout = 5 * time.Second
	rwTimeout   = 10 * time.Second
)

func handleTunneling(w http.ResponseWriter, r *http.Request) {
	destConn, err := net.DialTimeout("tcp", r.Host, dialTimeout)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close()

	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	go transfer(destConn, clientConn)
	go transfer(clientConn, destConn)
}
func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()

	reader := bufio.NewReader(source)
	writer := bufio.NewWriter(destination)

	buf := make([]byte, 64*1024)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Error reading from source: %v", err)
			}
			return
		}

		_, err = writer.Write(buf[:n])
		if err != nil {
			log.Printf("Error writing to destination: %v", err)
			return
		}
		writer.Flush()
	}
}

func handleHTTP(w http.ResponseWriter, req *http.Request) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func StartProxy() {
	server := &http.Server{
		Addr: ":3128",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleTunneling(w, r)
			} else {
				handleHTTP(w, r)
			}
		}),
	}
	log.Printf("Starting http/https proxy server on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
