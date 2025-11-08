package proxy

import (
	"net"
	"sync"
	"time"
)

func ValidateProxies(proxies []string) (validProxiesSl, invalidProxiesSl []string) {
	validProxies := make(chan string, len(proxies))
	invalidProxies := make(chan string, len(proxies))
	timeout := 5 * time.Second

	wg := &sync.WaitGroup{}
	wg.Add(len(proxies))

	for _, proxy := range proxies {
		go func(proxy string, timeout time.Duration) {
			defer wg.Done()
			if testProxy(proxy, timeout) {
				validProxies <- proxy
			} else {
				invalidProxies <- proxy
			}
		}(proxy, timeout)
	}

	wg.Wait()
	close(validProxies)
	close(invalidProxies)

	for proxy := range validProxies {
		validProxiesSl = append(validProxiesSl, proxy)
	}
	for proxy := range invalidProxies {
		invalidProxiesSl = append(invalidProxiesSl, proxy)
	}

	return validProxiesSl, invalidProxiesSl
}

func testProxy(proxy string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", proxy, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
