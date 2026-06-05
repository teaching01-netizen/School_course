package legacysync

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

func NewClient(baseURL, username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (c *Client) Login() error {
	token, err := c.fetchRequestToken()
	if err != nil {
		return fmt.Errorf("fetch token: %w", err)
	}

	form := url.Values{
		"Input.UserName": {c.username},
		"Input.Password": {c.password},
		"__RequestVerificationToken": {token},
	}

	loginURL := c.baseURL + "/Account/Login"
	resp, err := c.httpClient.PostForm(loginURL, form)
	if err != nil {
		return fmt.Errorf("login post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: status %d", resp.StatusCode)
	}

	// Check for login failure indicators in response body
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "Invalid login attempt") {
		return fmt.Errorf("login failed: invalid credentials")
	}

	return nil
}

func (c *Client) FetchSchedulePage(legacyCourseID string) (string, error) {
	detailURL := fmt.Sprintf("%s/Admin/Courses/Detail?id=%s", c.baseURL, url.QueryEscape(legacyCourseID))
	resp, err := c.httpClient.Get(detailURL)
	if err != nil {
		return "", fmt.Errorf("fetch schedule page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch schedule page: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return string(body), nil
}

func (c *Client) fetchRequestToken() (string, error) {
	loginURL := c.baseURL + "/Account/Login"
	resp, err := c.httpClient.Get(loginURL)
	if err != nil {
		return "", fmt.Errorf("get login page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get login page: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read login page: %w", err)
	}

	token := extractRequestToken(string(body))
	if token == "" {
		return "", fmt.Errorf("__RequestVerificationToken not found in login page")
	}
	return token, nil
}

func extractRequestToken(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	var token string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if token != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "input" {
			var name, value string
			for _, a := range n.Attr {
				if a.Key == "name" && a.Val == "__RequestVerificationToken" {
					name = a.Val
				}
				if a.Key == "value" {
					value = a.Val
				}
			}
			if name == "__RequestVerificationToken" {
				token = value
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return token
}
