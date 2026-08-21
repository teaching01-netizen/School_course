package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

func (c *Client) request(ctx context.Context, method, path string, query, form url.Values) (Response, error) {
	if path != "/Account/Login" || method != http.MethodPost {
		if !allowed(Request{Method: method, Path: path, Query: query, Form: form}) {
			return Response{}, ErrUnsafeEndpoint
		}
	}
	if err := c.reserveBudget(); err != nil {
		return Response{}, err
	}
	defer c.releaseBudgetEstimate()
	target, err := url.Parse(c.baseURL + path)
	if err != nil {
		return Response{}, fmt.Errorf("build source URL: %w", err)
	}
	target.RawQuery = query.Encode()
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(form.Encode())
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return Response{}, fmt.Errorf("build source request: %w", err)
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if err := c.waitForRequestSlot(ctx); err != nil {
		return Response{}, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsafeEndpoint):
			return Response{}, ErrUnsafeEndpoint
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return Response{}, fmt.Errorf("source request: %w", err)
		default:
			return Response{}, ErrSourceUnavailable
		}
	}
	defer response.Body.Close()
	if response.ContentLength > 0 && response.ContentLength > c.maxBodyBytes {
		// Abort before reading the body: no body bytes are counted against
		// the egress budget (the admission reservation is released by the
		// caller's defer).
		return Response{}, ErrResponseTooLarge
	}
	limited := io.LimitReader(response.Body, c.maxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Response{}, fmt.Errorf("read source response: %w", err)
		}
		return Response{}, ErrSourceUnavailable
	}
	c.recordEgressBytes(len(data))
	if int64(len(data)) > c.maxBodyBytes {
		return Response{}, ErrResponseTooLarge
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if contentType != "" && !strings.HasPrefix(contentType, "text/html") && !strings.HasPrefix(contentType, "text/plain") {
		return Response{}, ErrUnexpectedContentType
	}
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(response.Header.Get("Retry-After"))
		return Response{}, &RateLimitedError{RetryAfter: retryAfter, StatusCode: response.StatusCode}
	}
	if response.StatusCode >= http.StatusInternalServerError {
		return Response{}, ErrSourceUnavailable
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Response{}, &StatusError{StatusCode: response.StatusCode, Path: path}
	}
	return Response{StatusCode: response.StatusCode, Path: path, Body: data}, nil
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

func (c *Client) waitForRequestSlot(ctx context.Context) error {
	c.rateMu.Lock()
	prevNext := c.nextRequestAt
	now := time.Now()
	start := now
	if c.nextRequestAt.After(now) {
		start = c.nextRequestAt
	}
	c.nextRequestAt = start.Add(c.minRequestInterval)
	wait := start.Sub(now)
	c.rateMu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		c.rateMu.Lock()
		if c.nextRequestAt.Equal(start.Add(c.minRequestInterval)) {
			c.nextRequestAt = prevNext
		}
		c.rateMu.Unlock()
		return ctx.Err()
	}
}

func requestToken(body []byte) (string, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	var token string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if token != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "input" && attr(node, "name") == "__RequestVerificationToken" {
			token = attr(node, "value")
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	if token == "" {
		return "", ErrAuthentication
	}
	return token, nil
}

func isLoginPage(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, `action="/account/login"`) || strings.Contains(text, "name=\"__requestverificationtoken\"") && strings.Contains(text, "login")
}

func bytesContainFold(body []byte, needle string) bool {
	return strings.Contains(strings.ToLower(string(body)), strings.ToLower(needle))
}

func attr(node *html.Node, key string) string {
	for _, item := range node.Attr {
		if item.Key == key {
			return item.Val
		}
	}
	return ""
}
