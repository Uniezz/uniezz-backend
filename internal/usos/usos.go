// Package usos is a GO client for the USOS API, which is used for authentication and user information retrieval in the University Study-Oriented System (USOS).
package usos

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	ID            string            `json:"id"`
	FirstName     string            `json:"first_name"`
	LastName      string            `json:"last_name"`
	Email         *string           `json:"email"`
	StudentStatus *StudentStatus    `json:"student_status"`
	HasPhoto      bool              `json:"has_photo"`
	PhotoURLs     map[string]string `json:"photo_urls"`

	Programmes []StudentProgramme `json:"-"`
}

type LangDict struct {
	PL string `json:"pl"`
	EN string `json:"en"`
}

func (d LangDict) String() string {
	if d.PL != "" {
		return d.PL
	}
	return d.EN
}

type StudentProgramme struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	IsPrimary bool      `json:"is_primary"`
	Programme Programme `json:"programme"`
}

type Programme struct {
	ID            string   `json:"id"`
	Name          LangDict `json:"name"`
	Level         string   `json:"level"`
	ModeOfStudies LangDict `json:"mode_of_studies"`
	Faculty       *Faculty `json:"faculty"`
}

type Faculty struct {
	ID   string   `json:"id"`
	Name LangDict `json:"name"`
}

const (
	userFields      = "id|first_name|last_name|email|student_status|has_photo|photo_urls[400x400]"
	programmeFields = "id|status|is_primary|programme[id|name|level|mode_of_studies|faculty[id|name]]"
)

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
	config := c.newConfig("email", "studies")

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

	var user User
	if err := getJSON(ctx, authedClient, c.baseURL+"/services/users/user?fields="+url.QueryEscape(userFields), &user); err != nil {
		return User{}, fmt.Errorf("failed to fetch user info: %w", err)
	}

	err = getJSON(ctx, authedClient, c.baseURL+"/services/progs/student?fields="+url.QueryEscape(programmeFields), &user.Programmes)
	if err != nil {
		slog.WarnContext(ctx, "fetch USOS student programmes", slog.String("usos_user_id", user.ID), slog.Any("error", err))
		user.Programmes = nil
	}

	return user, nil
}

func getJSON(ctx context.Context, client *http.Client, target string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("usos api returned status: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}
