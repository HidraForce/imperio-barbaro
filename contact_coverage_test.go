package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestContactCoverageAcrossDatabases(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprint("postgres=", postgres), func(t *testing.T) {
			db := integrationDatabase(t, postgres)
			ctx := context.Background()
			person := contact{RealName: "Private person", Phone: "5511999991234", Notes: "Private notes", Accounts: []contactAccount{{PlayerTag: "#P0Y8LQ", PlayerName: "Main", ClanID: "1"}, {PlayerTag: "#QY8L0P", PlayerName: "Second", ClanID: "2"}}}
			without := contact{RealName: "No number", Accounts: []contactAccount{{PlayerTag: "#LGC", PlayerName: "Other", ClanID: "1"}}}
			for _, c := range []*contact{&person, &without} {
				if err := writeContact(ctx, db, c); err != nil {
					t.Fatal(err)
				}
			}
			a := &app{db: db}
			handler := adminTestRouter(a)
			if result := adminTestRequest(handler, "GET", "https://dashboard.imperiobarbaro.com.br/api/contacts/coverage", nil, false); result.Code != 401 {
				t.Fatal("coverage accessible without admin login", result.Code)
			}
			read := func() map[string]bool {
				t.Helper()
				result := adminTestRequest(handler, "GET", "https://dashboard.imperiobarbaro.com.br/api/contacts/coverage", nil, true)
				if result.Code != 200 {
					t.Fatal(result.Code, result.Body.String())
				}
				if result.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("coverage should not be cached")
				}
				for _, secret := range []string{person.Phone, person.RealName, person.Notes, "phone", "realName", "clanId"} {
					if secret != "" && strings.Contains(result.Body.String(), secret) {
						t.Fatal("coverage exposed private fields", secret)
					}
				}
				var data struct {
					Items map[string]bool `json:"items"`
				}
				if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil {
					t.Fatal(err)
				}
				return data.Items
			}
			items := read()
			if len(items) != 3 || !items["#P0Y8LQ"] || !items["#QY8L0P"] || items["#LGC"] {
				t.Fatal("multi-account cross-clan coverage incorrect", items)
			}
			person.Phone = ""
			if err := writeContact(ctx, db, &person); err != nil {
				t.Fatal(err)
			}
			if items = read(); items["#P0Y8LQ"] || items["#QY8L0P"] {
				t.Fatal("phone removal not reflected", items)
			}
			if err := eraseContact(ctx, db, person.ID); err != nil {
				t.Fatal(err)
			}
			if items = read(); len(items) != 1 {
				t.Fatal("deleted accounts retained", items)
			}
		})
	}
}
