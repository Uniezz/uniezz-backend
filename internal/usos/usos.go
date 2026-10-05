package usos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dghubble/oauth1"
)

type RequestToken struct {
	Token  string
	Secret string
}

type StudentStatus int

const (
	StatusNotStudent      StudentStatus = 0
	StatusInactiveStudent StudentStatus = 1
	StatusActiveStudent   StudentStatus = 2
)

type User struct {
	ID            string         `json:"id"`
	FirstName     string         `json:"first_name"`
	LastName      string         `json:"last_name"`
	Email         *string        `json:"email"`
	StudentStatus *StudentStatus `json:"student_status"`
}

type Client struct {
	baseURL        string
	consumerKey    string
	consumerSecret string
	callbackURL    string

	httpClient *http.Client
}

func NewClient(baseURL, consumerKey, consumerSecret, callbackURL string) *Client {
	return &Client{
		baseURL:        baseURL,
		consumerKey:    consumerKey,
		consumerSecret: consumerSecret,
		callbackURL:    callbackURL,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) newConfig(scopes ...string) *oauth1.Config {
	reqTokenURL := c.baseURL + "/services/oauth/request_token"
	if len(scopes) > 0 {
		reqTokenURL += "?scopes=" + url.QueryEscape(strings.Join(scopes, "|"))
	}

	usosEndpoint := oauth1.Endpoint{
		RequestTokenURL: reqTokenURL,
		AuthorizeURL:    c.baseURL + "/services/oauth/authorize",
		AccessTokenURL:  c.baseURL + "/services/oauth/access_token",
	}

	return &oauth1.Config{
		ConsumerKey:    c.consumerKey,
		ConsumerSecret: c.consumerSecret,
		CallbackURL:    c.callbackURL,
		Endpoint:       usosEndpoint,
		HTTPClient:     c.httpClient,
	}
}

func (c *Client) Begin(ctx context.Context) (RequestToken, string, error) {
	config := c.newConfig("email")

	requestToken, requestSecret, err := config.RequestToken()
	if err != nil {
		return RequestToken{}, "", fmt.Errorf("request token error: %w", err)
	}

	authorizationURL, err := config.AuthorizationURL(requestToken)
	if err != nil {
		return RequestToken{}, "", fmt.Errorf("authorization url error: %w", err)
	}

	return RequestToken{
		Token:  requestToken,
		Secret: requestSecret,
	}, authorizationURL.String(), nil
}

func (c *Client) Finish(ctx context.Context, rt RequestToken, verifier string) (User, error) {
	config := c.newConfig()
	accessToken, accessSecret, err := config.AccessToken(rt.Token, rt.Secret, verifier)
	if err != nil {
		return User{}, fmt.Errorf("failed to exchange access token: %w", err)
	}

	token := oauth1.NewToken(accessToken, accessSecret)
	ctx = context.WithValue(ctx, oauth1.HTTPClient, c.httpClient)
	authedClient := config.Client(ctx, token)

	baseURL := c.baseURL
	userURL := fmt.Sprintf("%s/services/users/user?fields=id|first_name|last_name|email|student_status", baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return User{}, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := authedClient.Do(req)
	if err != nil {
		return User{}, fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("usos api returned status: %d", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return User{}, fmt.Errorf("failed to decode user response: %w", err)
	}

	return user, nil
}
