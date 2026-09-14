package service

import (
	"errors"
	"net/http"
	"net/url"
	"time"
)

const mediaVideoMaxProxyClients = 64

type mediaVideoProxyClient struct {
	client    *http.Client
	transport *http.Transport
	lastUsed  time.Time
}

func newMediaVideoTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	transport := base.Clone()
	transport.IdleConnTimeout = 90 * time.Second
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 8
	return transport
}

func newMediaVideoHTTPClient(transport *http.Transport) *http.Client {
	// Each operation has its own context deadline. Keep this ceiling above
	// the configurable download duration without extending create/poll deadlines.
	return &http.Client{Timeout: 2 * time.Hour, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Cache by the complete proxy URL, including credentials, so different proxy
// identities never share tunnels. Bound the cache even if proxies keep changing.
func (s *MediaVideoService) proxyHTTPClient(rawURL string) (*http.Client, error) {
	s.httpMu.Lock()
	defer s.httpMu.Unlock()
	if s.httpStopped {
		return nil, errors.New("media video service is stopped")
	}
	if entry := s.proxyClients[rawURL]; entry != nil {
		entry.lastUsed = time.Now()
		return entry.client, nil
	}
	proxyURL, err := url.Parse(rawURL)
	if err != nil || proxyURL.Host == "" {
		// Do not include credentials in errors or silently bypass a bad proxy.
		return nil, errors.New("invalid media video proxy URL")
	}
	switch proxyURL.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New("unsupported media video proxy scheme")
	}
	if s.proxyClients == nil {
		s.proxyClients = make(map[string]*mediaVideoProxyClient)
	}
	if len(s.proxyClients) >= mediaVideoMaxProxyClients {
		var oldestKey string
		var oldest *mediaVideoProxyClient
		for key, entry := range s.proxyClients {
			if oldest == nil || entry.lastUsed.Before(oldest.lastUsed) {
				oldestKey, oldest = key, entry
			}
		}
		oldest.transport.CloseIdleConnections()
		delete(s.proxyClients, oldestKey)
	}
	transport := newMediaVideoTransport()
	transport.Proxy = http.ProxyURL(proxyURL)
	client := newMediaVideoHTTPClient(transport)
	s.proxyClients[rawURL] = &mediaVideoProxyClient{client, transport, time.Now()}
	return client, nil
}

func (s *MediaVideoService) closeHTTPConnections() {
	s.httpMu.Lock()
	defer s.httpMu.Unlock()
	s.httpStopped = true
	for key, entry := range s.proxyClients {
		entry.transport.CloseIdleConnections()
		delete(s.proxyClients, key)
	}
	if s.ownedTransport != nil {
		s.ownedTransport.CloseIdleConnections()
	}
}
