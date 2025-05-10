package httpproxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func handleTunneling(w http.ResponseWriter, r *http.Request) {
	dialer := net.Dialer{Timeout: 30 * time.Second} // 例如30秒超时
	destConn, err := dialer.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close() // 确保destConn被关闭

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
	defer clientConn.Close() // 确保clientConn被关闭

	var wg sync.WaitGroup
	wg.Add(2)
	go transfer(destConn, clientConn, &wg)
	go transfer(clientConn, destConn, &wg)
	wg.Wait() // 等待所有转移完成
}

func transfer(destination io.WriteCloser, source io.ReadCloser, wg *sync.WaitGroup) {
	defer wg.Done()
	// 或者，强制使用 buffer
	_, _ = io.Copy(destination, source)
	if tcpConn, ok := destination.(interface{ CloseWrite() error }); ok {
		_ = tcpConn.CloseWrite()
	}
}

var transport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment, // 使用环境变量中的代理
	DialContext:           (&net.Dialer{Timeout: 60 * time.Second}).DialContext,
	TLSHandshakeTimeout:   30 * time.Second,
	ResponseHeaderTimeout: 300 * time.Second,
	ExpectContinueTimeout: 300 * time.Second,
	MaxIdleConns:          24,
	IdleConnTimeout:       60 * time.Second,
}

func handleHTTP(w http.ResponseWriter, req *http.Request) {
	// 使用自定义的 Transport，设置超时等参数
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36")
	// 添加X-Forwarded-For头
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send request to destination: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	copyHeader(w.Header(), resp.Header)

	// 设置状态码
	w.WriteHeader(resp.StatusCode)

	// 使用 io.Copy 复制响应体，并处理错误
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying response body: %v", err)
	}
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
		Addr: ":31280",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				handleTunneling(w, r)
			} else {
				handleHTTP(w, r)
			}
		}),
		ReadTimeout:  360 * time.Second, // 设置读取超时
		WriteTimeout: 360 * time.Second, // 设置写入超时
	}
	log.Printf("Starting http/https proxy server on %s", server.Addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		// 忽略 ErrServerClosed 错误，这是正常的关闭行为
		log.Fatalf("Proxy server error: %v", err)
	}
}
