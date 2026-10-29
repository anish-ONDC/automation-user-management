package ondc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Shape copied from a real UAT exchange on 2026-10-27 (subscriber list trimmed).
const uatSuccess = `{"status":"success","requestId":null,"data":{"user_id":25716,"email":"uat_1@yopmail.com","first_name":"UAT","last_name":"1","status":1,"organisations":[{"organisation_id":11899,"organisation_name":"UAT_1 Org","network_status":3,"participant_id":"11899.participant.ondc.org","np_type":"Admin","is_representative":true,"status":1,"subscriber_ids":["bap-preprod.martpe.in","buyer-app-preprod-v2.ondc.org"]}]}}`

const uatUsedCode = `{"status":"error","code":400,"error":{"message":{"err_msg":"Invalid, expired, or already used authorization code."},"is_cross_org_conflict":false}}`

func server(t *testing.T, status int, body string, check func(map[string]string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != exchangePath {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if check != nil {
			raw, _ := io.ReadAll(r.Body)
			var got map[string]string
			_ = json.Unmarshal(raw, &got)
			check(got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestExchange_SuccessMatchesUATShape(t *testing.T) {
	srv := server(t, 200, uatSuccess, func(got map[string]string) {
		if got["client_id"] != "cid" || got["client_secret"] != "sec" || got["code"] != "abc" {
			t.Errorf("request body = %v", got)
		}
		if _, extra := got["redirect_uri"]; extra {
			t.Error("guide says redirect_uri must not be sent")
		}
	})
	defer srv.Close()

	info, err := NewClient(srv.URL+"/", "cid", "sec").Exchange(context.Background(), "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.UserID != 25716 || info.Email != "uat_1@yopmail.com" || len(info.Organisations) != 1 {
		t.Fatalf("unexpected user: %+v", info)
	}
	org := info.Organisations[0]
	if org.OrgID() != "11899" || org.NetworkStatus != "3" || org.NPType != "Admin" || len(org.SubscriberIDs) != 2 {
		t.Fatalf("unexpected organisation: %+v", org)
	}
}

func TestFlexString_AcceptsStringAndNumber(t *testing.T) {
	for in, want := range map[string]FlexString{`"3"`: "3", `3`: "3", `null`: ""} {
		var f FlexString
		if err := json.Unmarshal([]byte(in), &f); err != nil || f != want {
			t.Errorf("%s -> %q, %v; want %q", in, f, err, want)
		}
	}
}

func TestExchange_ErrorKinds(t *testing.T) {
	cases := []struct {
		status int
		body   string
		kind   string
	}{
		{400, uatUsedCode, KindInvalidCode},
		{422, `{}`, KindBadRequest},
		{429, `{}`, KindRateLimited},
		{503, `oops`, KindUnavailable},
		{200, `{"status":"success","data":{}}`, KindBadResponse},
	}
	for _, tc := range cases {
		srv := server(t, tc.status, tc.body, nil)
		_, err := NewClient(srv.URL, "cid", "sec").Exchange(context.Background(), "abc")
		srv.Close()
		e, ok := IsExchangeError(err)
		if !ok || e.Kind != tc.kind {
			t.Errorf("http %d: got %v, want kind %s", tc.status, err, tc.kind)
		}
	}
}

func TestExchange_UsedCodeKeepsONDCMessage(t *testing.T) {
	srv := server(t, 400, uatUsedCode, nil)
	defer srv.Close()
	_, err := NewClient(srv.URL, "cid", "sec").Exchange(context.Background(), "abc")
	e, _ := IsExchangeError(err)
	if e == nil || !strings.Contains(e.Message, "already used") {
		t.Fatalf("expected ONDC's message, got %v", err)
	}
	if strings.Contains(err.Error(), "sec") {
		t.Fatal("error text must never contain the client secret")
	}
}

func TestValidCode(t *testing.T) {
	good := "3cc2a1b0f4e5d6c7b8a9f0e1d2c3b4a5f6e7d8c9b0a1f2e3d4c5b6a7f8e9d0c1"
	if !ValidCode(good) {
		t.Error("real-shaped code rejected")
	}
	for _, bad := range []string{"", "abc123", "this-code-is-fake-and-should-fail-000000", "has space in it 123456", "<script>alert(1)</script>", strings.Repeat("a", 600)} {
		if ValidCode(bad) {
			t.Errorf("accepted bad code %q", bad)
		}
	}
}

func TestLoginURL(t *testing.T) {
	got := LoginURL("https://ondc-uat-ui.kellton.net/", "4e125e1e-a97c")
	if got != "https://ondc-uat-ui.kellton.net/sso-client?clientId=4e125e1e-a97c" {
		t.Fatalf("got %s", got)
	}
}
