package proxy

import (
	"crypto/tls"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
)

type ProxyRotator struct {
	proxies []string
	current uint32
}

func NewProxyRotator(proxies []string) *ProxyRotator {
	return &ProxyRotator{proxies: proxies}
}

func (p *ProxyRotator) Next() string {
	if len(p.proxies) == 0 {
		return ""
	}
	n := atomic.AddUint32(&p.current, 1)
	return p.proxies[(int(n)-1)%len(p.proxies)]
}

func (p *ProxyRotator) GetClient() *fasthttp.Client {
	return &fasthttp.Client{
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		MaxConnDuration: 30 * time.Second,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		Dial: func(addr string) (net.Conn, error) {
			proxy := p.Next()
			if proxy == "" {
				return nil, fmt.Errorf("proxy address is empty")
			}
			return fasthttpproxy.FasthttpSocksDialer("socks5://" + proxy)(addr)
		},
	}
}
