package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestThirdClanIsSelectableAcrossReports(t *testing.T) {
	t.Setenv("CLAN_TAG1", "")
	t.Setenv("CLAN_TAG2", "")
	t.Setenv("CLAN_TAG3", "")
	if tags := configuredClanTags(); tags["3"] != "" {
		t.Fatal("an explicitly empty tag should disable Clash API calls for clan 3")
	}
	if err := os.Unsetenv("CLAN_TAG3"); err != nil {
		t.Fatal(err)
	}
	a := adminTestApp(t)
	a.clanTags = configuredClanTags()
	a.token = "test-token"
	res := httptest.NewRecorder()
	a.config(res, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var config struct {
		Configured bool
		Clans      []struct {
			ID, Tag    string
			Configured bool
		}
	}
	if err := json.Unmarshal(res.Body.Bytes(), &config); err != nil || !config.Configured || len(config.Clans) != 3 {
		t.Fatalf("third clan missing from configuration: %s", res.Body)
	}
	if third := config.Clans[2]; third.ID != "3" || third.Tag != "2CJR2VL8R" || !third.Configured {
		t.Fatalf("invalid third clan configuration: %+v", third)
	}
	for _, tc := range []struct {
		name, query, apiPath, body string
		handler                    http.HandlerFunc
	}{
		{"roster", "?clan=3", "", `{"tag":"#2CJR2VL8R","memberList":[{"tag":"#P0Y8LQ","name":"Jogador"}]}`, a.clan},
		{"war", "", "/currentwar", `{"state":"notInWar"}`, a.war},
		{"raids", "?clan=3", "/capitalraidseasons", `{"items":[]}`, a.raids},
		{"league", "?clan=3", "/currentwar/leaguegroup", `{"state":"inWar","rounds":[]}`, a.cwl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.clash = &http.Client{Transport: clashTransport(func(r *http.Request) (*http.Response, error) {
				if want := "/v1/clans/%232CJR2VL8R" + tc.apiPath; r.URL.EscapedPath() != want {
					t.Errorf("Clash path %q, want %q", r.URL.EscapedPath(), want)
				}
				return clashResponse(200, tc.body), nil
			})}
			res := httptest.NewRecorder()
			tc.handler(res, httptest.NewRequest(http.MethodGet, "/api/"+tc.name+tc.query, nil))
			if res.Code != http.StatusOK {
				t.Fatalf("report failed: %d %s", res.Code, res.Body)
			}
		})
	}
}
