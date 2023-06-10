package httpproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func Check(proxyAddr string, timeout time.Duration) (string, error) {
	proxyURL, err := parseURLWithDefaultScheme(proxyAddr, "http")
	if err != nil {
		return "", err
	}
	hc := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}
	resp, err := hc.Get("http://ipinfo.io/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status code:%d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	type Response struct {
		IP string `json:"ip"`
	}

	var r Response
	err = json.Unmarshal(b, &r)
	if err != nil {
		return "", err
	}

	return r.IP, nil
}

func parseURLWithDefaultScheme(rawURL string, defaultScheme string) (*url.URL, error) {
	if !strings.Contains(rawURL, "://") {
		rawURL = fmt.Sprintf("%s://%s", defaultScheme, rawURL)
	}
	return url.Parse(rawURL)
}
