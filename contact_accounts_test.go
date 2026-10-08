package main

import "testing"

func TestPersonCanHaveMultipleAccountsInBothClans(t *testing.T) {
	c := contact{RealName: " Bruno ", Phone: "+55 11 99999-0000", Accounts: []contactAccount{
		{PlayerTag: "p0y8lq", PlayerName: "Principal", ClanID: "1"},
		{PlayerTag: "#QY8L0P", PlayerName: "Secundária", ClanID: "2"},
		{PlayerTag: "#GRJCUV", PlayerName: "Terceira", ClanID: "1"},
	}}
	if err := validateContact(&c); err != nil {
		t.Fatal(err)
	}
	if c.RealName != "Bruno" || c.Accounts[0].PlayerTag != "#P0Y8LQ" || len(c.Accounts) != 3 {
		t.Fatal("normalização incorreta")
	}
}

func TestPersonAccountsRejectDuplicatesInvalidClansAndEmptyNames(t *testing.T) {
	for _, accounts := range [][]contactAccount{
		{},
		{{PlayerTag: "#P0Y8LQ", PlayerName: "Principal"}, {PlayerTag: "p0y8lq", PlayerName: "Duplicada"}},
		{{PlayerTag: "#P0Y8LQ", PlayerName: "Principal", ClanID: "3"}},
		{{PlayerTag: "#P0Y8LQ", PlayerName: ""}},
	} {
		c := contact{RealName: "Pessoa", Accounts: accounts}
		if validateContact(&c) == nil {
			t.Errorf("contas inválidas aceitas: %+v", accounts)
		}
	}
}

func TestLegacySingleAccountContactRemainsSupported(t *testing.T) {
	c := contact{PlayerTag: "#P0Y8LQ", PlayerName: "Nome no jogo", Phone: "+55 11 99999-0000"}
	if err := validateContact(&c); err != nil {
		t.Fatal(err)
	}
	if c.RealName != "Nome no jogo" || len(c.Accounts) != 1 || c.Accounts[0].ClanID != "" {
		t.Fatal("cadastro antigo não foi preservado")
	}
}
