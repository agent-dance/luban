package web

import (
	"net/http"
	"net/url"
)

type webFetchTestRoundTripper func(*http.Request) (*http.Response, error)

func (f webFetchTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func localWebFetchTestClient() *http.Client {
	transport := http.DefaultTransport
	return &http.Client{
		Timeout: WebFetchTimeout,
		Transport: webFetchTestRoundTripper(func(request *http.Request) (*http.Response, error) {
			clone := request.Clone(request.Context())
			urlCopy := *request.URL
			urlCopy.Scheme = "http"
			urlCopy.Host = "127.0.0.1:" + request.URL.Port()
			clone.URL = &urlCopy
			response, err := transport.RoundTrip(clone)
			if response != nil {
				response.Request = request
			}
			return response, err
		}),
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func localWebFetchTestURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsed.Scheme = "https"
	parsed.Host = "8.8.8.8:" + parsed.Port()
	return parsed.String()
}
