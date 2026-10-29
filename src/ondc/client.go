// Package ondc talks to ONDC SSO ("Login via ONDC", SSO Onboarding Guide v1.2):
// it builds the login redirect and swaps the one-time callback code for the
// user's details. The client secret only ever travels server to server.
package ondc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const exchangePath = "/api/auth/sso/user-info/exchange"

// ONDC rejects codes that are not hexadecimal (422), and UAT codes are 64 hex
// characters. The length range leaves room if ONDC changes the size.
var codePattern = regexp.MustCompile(`^[0-9a-fA-F]{16,512}$`)

// ValidCode rejects a missing or malformed code before calling ONDC.
func ValidCode(code string) bool {
	return codePattern.MatchString(code)
}

// LoginURL is where the browser is sent to log in. The client ID is not a secret.
func LoginURL(portalBaseURL, clientID string) string {
	return strings.TrimRight(portalBaseURL, "/") + "/sso-client?clientId=" + url.QueryEscape(clientID)
}

// FlexString accepts a JSON string or number. UAT sends network_status as a
// number (3) while the guide's example shows a string ("3").
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = FlexString(n.String())
	return nil
}

type Organisation struct {
	OrganisationID   int64      `json:"organisation_id"`
	OrganisationName string     `json:"organisation_name"`
	NetworkStatus    FlexString `json:"network_status"`
	ParticipantID    string     `json:"participant_id"`
	NPType           string     `json:"np_type"`
	IsRepresentative bool       `json:"is_representative"`
	Status           int        `json:"status"`
	SubscriberIDs    []string   `json:"subscriber_ids"`
}

type UserInfo struct {
	UserID        int64          `json:"user_id"`
	Email         string         `json:"email"`
	FirstName     string         `json:"first_name"`
	LastName      string         `json:"last_name"`
	Status        int            `json:"status"`
	Organisations []Organisation `json:"organisations"`
}

// OrgID is the organisation ID as a string, or "" when the user has none.
func (o Organisation) OrgID() string {
	if o.OrganisationID == 0 {
		return ""
	}
	return strconv.FormatInt(o.OrganisationID, 10)
}

// ExchangeError says why ONDC refused or failed the exchange. Message is ONDC's
// own err_msg and never contains the code or the secret, so it is safe to log.
type ExchangeError struct {
	Status  int // HTTP status from ONDC; 0 for network errors
	Kind    string
	Message string
}

func (e *ExchangeError) Error() string {
	return fmt.Sprintf("ondc exchange failed: %s (http %d): %s", e.Kind, e.Status, e.Message)
}

const (
	KindInvalidCode = "invalid_or_used_code" // 400: invalid credentials, expired or already-used code
	KindBadRequest  = "bad_request"          // 422: missing or malformed field in our request
	KindRateLimited = "rate_limited"         // 429
	KindUnavailable = "unavailable"          // 5xx or network
	KindBadResponse = "bad_response"         // 200 but unusable body
)

type Client struct {
	APIBaseURL   string
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
}

func NewClient(apiBaseURL, clientID, clientSecret string) *Client {
	return &Client{
		APIBaseURL:   strings.TrimRight(apiBaseURL, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTP:         &http.Client{Timeout: 10 * time.Second},
	}
}

type exchangeResponse struct {
	Status string   `json:"status"`
	Data   UserInfo `json:"data"`
	Error  struct {
		Message struct {
			ErrMsg string `json:"err_msg"`
		} `json:"message"`
	} `json:"error"`
}

// Exchange swaps a one-time code for the user's details. The code is single
// use: a successful call consumes it.
func (c *Client) Exchange(ctx context.Context, code string) (*UserInfo, error) {
	body, err := json.Marshal(map[string]string{
		"client_id":     c.ClientID,
		"client_secret": c.ClientSecret,
		"code":          code,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIBaseURL+exchangePath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &ExchangeError{Kind: KindUnavailable, Message: "request to ONDC failed"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var parsed exchangeResponse
	_ = json.Unmarshal(raw, &parsed)

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusBadRequest:
		return nil, &ExchangeError{Status: resp.StatusCode, Kind: KindInvalidCode, Message: parsed.Error.Message.ErrMsg}
	case resp.StatusCode == http.StatusUnprocessableEntity:
		return nil, &ExchangeError{Status: resp.StatusCode, Kind: KindBadRequest, Message: parsed.Error.Message.ErrMsg}
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, &ExchangeError{Status: resp.StatusCode, Kind: KindRateLimited, Message: parsed.Error.Message.ErrMsg}
	default:
		return nil, &ExchangeError{Status: resp.StatusCode, Kind: KindUnavailable, Message: parsed.Error.Message.ErrMsg}
	}

	if parsed.Status != "success" || parsed.Data.UserID == 0 || parsed.Data.Email == "" {
		return nil, &ExchangeError{Status: resp.StatusCode, Kind: KindBadResponse, Message: "missing user_id or email"}
	}
	return &parsed.Data, nil
}

// IsExchangeError reports whether err came from ONDC rather than our own code.
func IsExchangeError(err error) (*ExchangeError, bool) {
	var e *ExchangeError
	ok := errors.As(err, &e)
	return e, ok
}
